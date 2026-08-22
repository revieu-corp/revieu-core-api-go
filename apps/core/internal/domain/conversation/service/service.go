package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

type ConversationService struct {
	db *gorm.DB
}

type ConversationSummary struct {
	ID            int64      `json:"id"`
	Type          string     `json:"type"`
	Title         string     `json:"title"`
	AvatarURL     string     `json:"avatar_url,omitempty"`
	LastMessage   string     `json:"last_message"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	UnreadCount   int64      `json:"unread_count"`
	IsMuted       bool       `json:"is_muted"`
}

type ConversationMessage struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	SenderID       int64     `json:"sender_id"`
	SenderName     string    `json:"sender_name"`
	SenderAvatar   string    `json:"sender_avatar,omitempty"`
	Content        string    `json:"content"`
	MessageType    string    `json:"message_type"`
	IsRead         bool      `json:"is_read"`
	CreatedAt      time.Time `json:"created_at"`
}

const (
	DefaultConversationMessageLimit = 50
	MaxConversationMessageLimit     = 100
)

// ConversationMessageListQuery controls message history pagination.
// Cursor is the oldest message id from the previous page; the next page
// contains older messages.
type ConversationMessageListQuery struct {
	Limit  int
	Cursor *int64
}

type CreateConversationInput struct {
	Title          string  `json:"title"`
	Type           string  `json:"type"`
	ParticipantIDs []int64 `json:"participant_ids"`
}

type SendMessageInput struct {
	Content     string `json:"content"`
	MessageType string `json:"message_type"`
}

type UpdateConversationSettingsInput struct {
	IsMuted *bool `json:"is_muted"`
}

var ErrConversationNotFound = errors.New("conversation not found")
var ErrConversationForbidden = errors.New("conversation forbidden")
var ErrConversationInvalidInput = errors.New("conversation invalid input")

func NewConversationService(db *gorm.DB) *ConversationService {
	if db == nil {
		db = database.DB
	}
	return &ConversationService{db: db}
}

func (s *ConversationService) List(ctx context.Context, userID int64) ([]ConversationSummary, error) {
	memberships, err := s.loadMemberships(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return []ConversationSummary{}, nil
	}

	conversationIDs := make([]int64, 0, len(memberships))
	membershipByConversation := make(map[int64]model.ConversationParticipant, len(memberships))
	for _, membership := range memberships {
		conversationIDs = append(conversationIDs, membership.ConversationID)
		membershipByConversation[membership.ConversationID] = membership
	}

	var conversations []model.Conversation
	if err := s.db.WithContext(ctx).
		Preload("Participants.User.Profile").
		Preload("Messages", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Where("id IN ?", conversationIDs).
		Order("updated_at desc").
		Find(&conversations).Error; err != nil {
		return nil, err
	}

	summaries := make([]ConversationSummary, 0, len(conversations))
	for _, conversation := range conversations {
		summary, err := s.buildConversationSummary(ctx, userID, conversation, membershipByConversation[conversation.ID])
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func (s *ConversationService) Create(ctx context.Context, userID int64, input CreateConversationInput) (*ConversationSummary, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, ErrConversationInvalidInput
	}

	conversationType := strings.TrimSpace(input.Type)
	if conversationType == "" {
		conversationType = "group"
	}

	participantIDs := uniqueParticipantIDs(append(input.ParticipantIDs, userID))
	conversation := model.Conversation{
		Type:  conversationType,
		Title: title,
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}

		participants := make([]model.ConversationParticipant, 0, len(participantIDs))
		for _, participantID := range participantIDs {
			role := "member"
			if participantID == userID {
				role = "owner"
			}
			participants = append(participants, model.ConversationParticipant{
				ConversationID: conversation.ID,
				UserID:         participantID,
				Role:           role,
				JoinedAt:       time.Now().UTC(),
			})
		}

		return tx.Create(&participants).Error
	}); err != nil {
		return nil, err
	}

	return &ConversationSummary{
		ID:          conversation.ID,
		Type:        conversation.Type,
		Title:       conversation.Title,
		LastMessage: "",
		UnreadCount: 0,
		IsMuted:     false,
	}, nil
}

func (s *ConversationService) Messages(ctx context.Context, userID, conversationID int64, query ConversationMessageListQuery) ([]ConversationMessage, *int64, error) {
	membership, err := s.membershipForUser(ctx, userID, conversationID)
	if err != nil {
		return nil, nil, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = DefaultConversationMessageLimit
	}
	if limit > MaxConversationMessageLimit {
		limit = MaxConversationMessageLimit
	}

	var messages []model.Message
	dbQuery := s.db.WithContext(ctx).
		Preload("Sender.Profile").
		Where("conversation_id = ?", conversationID).
		Order("id desc").
		Limit(limit + 1)
	if query.Cursor != nil {
		dbQuery = dbQuery.Where("id < ?", *query.Cursor)
	}
	if err := dbQuery.Find(&messages).Error; err != nil {
		return nil, nil, err
	}

	var nextCursor *int64
	if len(messages) > limit {
		value := messages[limit-1].ID
		nextCursor = &value
		messages = messages[:limit]
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}

	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).
		Model(&model.ConversationParticipant{}).
		Where("id = ?", membership.ID).
		Update("last_read_at", now).Error; err != nil {
		return nil, nil, err
	}
	if err := s.db.WithContext(ctx).
		Model(&model.Message{}).
		Where("conversation_id = ? AND sender_id <> ?", conversationID, userID).
		Update("is_read", true).Error; err != nil {
		return nil, nil, err
	}

	result := make([]ConversationMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, mapConversationMessage(message))
	}

	return result, nextCursor, nil
}

func (s *ConversationService) SendMessage(ctx context.Context, userID, conversationID int64, input SendMessageInput) (*ConversationMessage, error) {
	if strings.TrimSpace(input.Content) == "" {
		return nil, ErrConversationInvalidInput
	}

	if _, err := s.membershipForUser(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	messageType := strings.TrimSpace(input.MessageType)
	if messageType == "" {
		messageType = "text"
	}

	message := model.Message{
		ConversationID: conversationID,
		SenderID:       userID,
		Content:        strings.TrimSpace(input.Content),
		MessageType:    messageType,
		IsRead:         false,
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		return tx.Model(&model.Conversation{}).
			Where("id = ?", conversationID).
			Update("updated_at", message.CreatedAt).Error
	}); err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Preload("Sender.Profile").First(&message, message.ID).Error; err != nil {
		return nil, err
	}

	result := mapConversationMessage(message)
	return &result, nil
}

func (s *ConversationService) UpdateSettings(ctx context.Context, userID, conversationID int64, input UpdateConversationSettingsInput) (*ConversationSummary, error) {
	membership, err := s.membershipForUser(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}

	if input.IsMuted != nil {
		membership.IsMuted = *input.IsMuted
		if err := s.db.WithContext(ctx).Model(&membership).Update("is_muted", membership.IsMuted).Error; err != nil {
			return nil, err
		}
	}

	var conversation model.Conversation
	if err := s.db.WithContext(ctx).
		Preload("Participants.User.Profile").
		Preload("Messages", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		First(&conversation, conversationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	summary, err := s.buildConversationSummary(ctx, userID, conversation, membership)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *ConversationService) loadMemberships(ctx context.Context, userID int64) ([]model.ConversationParticipant, error) {
	var memberships []model.ConversationParticipant
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&memberships).Error; err != nil {
		return nil, err
	}
	return memberships, nil
}

func (s *ConversationService) membershipForUser(ctx context.Context, userID, conversationID int64) (model.ConversationParticipant, error) {
	var membership model.ConversationParticipant
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND conversation_id = ?", userID, conversationID).
		First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var conversationCount int64
			if countErr := s.db.WithContext(ctx).
				Model(&model.Conversation{}).
				Where("id = ?", conversationID).
				Count(&conversationCount).Error; countErr != nil {
				return membership, countErr
			}
			if conversationCount == 0 {
				return membership, ErrConversationNotFound
			}
			return membership, ErrConversationForbidden
		}
		return membership, err
	}
	return membership, nil
}

func (s *ConversationService) buildConversationSummary(
	ctx context.Context,
	userID int64,
	conversation model.Conversation,
	membership model.ConversationParticipant,
) (ConversationSummary, error) {
	var unreadCount int64
	query := s.db.WithContext(ctx).
		Model(&model.Message{}).
		Where("conversation_id = ? AND sender_id <> ?", conversation.ID, userID)
	if !membership.LastReadAt.IsZero() {
		query = query.Where("created_at > ?", membership.LastReadAt)
	}
	if err := query.Count(&unreadCount).Error; err != nil {
		return ConversationSummary{}, err
	}

	summary := ConversationSummary{
		ID:          conversation.ID,
		Type:        conversation.Type,
		Title:       conversation.Title,
		UnreadCount: unreadCount,
		IsMuted:     membership.IsMuted,
	}

	if len(conversation.Messages) > 0 {
		lastMessage := conversation.Messages[0]
		summary.LastMessage = lastMessage.Content
		summary.LastMessageAt = &lastMessage.CreatedAt
	}

	if summary.Title == "" {
		summary.Title = defaultConversationTitle(conversation.Participants, userID)
	}
	summary.AvatarURL = otherParticipantAvatar(conversation.Participants, userID)

	return summary, nil
}

func mapConversationMessage(message model.Message) ConversationMessage {
	return ConversationMessage{
		ID:             message.ID,
		ConversationID: message.ConversationID,
		SenderID:       message.SenderID,
		SenderName:     userDisplayName(message.Sender),
		SenderAvatar:   userAvatar(message.Sender),
		Content:        message.Content,
		MessageType:    message.MessageType,
		IsRead:         message.IsRead,
		CreatedAt:      message.CreatedAt,
	}
}

func defaultConversationTitle(participants []model.ConversationParticipant, currentUserID int64) string {
	for _, participant := range participants {
		if participant.UserID == currentUserID {
			continue
		}
		if participant.User != nil {
			return userDisplayName(participant.User)
		}
	}
	return "Conversation"
}

func otherParticipantAvatar(participants []model.ConversationParticipant, currentUserID int64) string {
	for _, participant := range participants {
		if participant.UserID == currentUserID {
			continue
		}
		if participant.User != nil {
			return userAvatar(participant.User)
		}
	}
	return ""
}

func userDisplayName(user *model.User) string {
	if user == nil {
		return "Unknown User"
	}
	if user.Profile != nil && strings.TrimSpace(user.Profile.Nickname) != "" {
		return user.Profile.Nickname
	}
	return "User"
}

func userAvatar(user *model.User) string {
	if user == nil || user.Profile == nil {
		return ""
	}
	return user.Profile.AvatarURL
}

func uniqueParticipantIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
