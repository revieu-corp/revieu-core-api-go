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

const (
	DefaultConversationListLimit = 20
	MaxConversationListLimit     = 100
)

// ConversationListQuery controls the authenticated conversation list.
// Cursor is the last conversation id returned by the previous page.
type ConversationListQuery struct {
	Limit  int
	Cursor *int64
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

func (s *ConversationService) List(ctx context.Context, userID int64, query ConversationListQuery) ([]ConversationSummary, *int64, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = DefaultConversationListLimit
	}
	if limit > MaxConversationListLimit {
		limit = MaxConversationListLimit
	}

	memberships, nextCursor, err := s.loadMemberships(ctx, userID, query.Cursor, limit)
	if err != nil {
		return nil, nil, err
	}
	if len(memberships) == 0 {
		return []ConversationSummary{}, nil, nil
	}

	conversationIDs := make([]int64, 0, len(memberships))
	for _, membership := range memberships {
		conversationIDs = append(conversationIDs, membership.ConversationID)
	}

	var conversations []model.Conversation
	if err := s.db.WithContext(ctx).
		Preload("Participants.User.Profile").
		Where("id IN ?", conversationIDs).
		Find(&conversations).Error; err != nil {
		return nil, nil, err
	}
	conversationByID := make(map[int64]model.Conversation, len(conversations))
	for _, conversation := range conversations {
		conversationByID[conversation.ID] = conversation
	}

	latestMessages, err := s.loadLatestMessages(ctx, conversationIDs)
	if err != nil {
		return nil, nil, err
	}
	unreadCounts, err := s.loadUnreadCounts(ctx, userID, conversationIDs)
	if err != nil {
		return nil, nil, err
	}

	summaries := make([]ConversationSummary, 0, len(conversations))
	for _, membership := range memberships {
		conversation, exists := conversationByID[membership.ConversationID]
		if !exists {
			continue
		}
		summary := s.buildConversationSummary(
			userID,
			conversation,
			membership,
			latestMessages[conversation.ID],
			unreadCounts[conversation.ID],
		)
		if summary.ID == 0 {
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nextCursor, nil
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

func (s *ConversationService) Messages(ctx context.Context, userID, conversationID int64) ([]ConversationMessage, error) {
	membership, err := s.membershipForUser(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}

	var messages []model.Message
	if err := s.db.WithContext(ctx).
		Preload("Sender.Profile").
		Where("conversation_id = ?", conversationID).
		Order("created_at asc, id asc").
		Find(&messages).Error; err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).
		Model(&model.ConversationParticipant{}).
		Where("id = ?", membership.ID).
		Update("last_read_at", now).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Model(&model.Message{}).
		Where("conversation_id = ? AND sender_id <> ?", conversationID, userID).
		Update("is_read", true).Error; err != nil {
		return nil, err
	}

	result := make([]ConversationMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, mapConversationMessage(message))
	}

	return result, nil
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
		First(&conversation, conversationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	latestMessages, err := s.loadLatestMessages(ctx, []int64{conversationID})
	if err != nil {
		return nil, err
	}
	unreadCounts, err := s.loadUnreadCounts(ctx, userID, []int64{conversationID})
	if err != nil {
		return nil, err
	}
	summary := s.buildConversationSummary(userID, conversation, membership, latestMessages[conversationID], unreadCounts[conversationID])
	return &summary, nil
}

func (s *ConversationService) loadMemberships(ctx context.Context, userID int64, cursor *int64, limit int) ([]model.ConversationParticipant, *int64, error) {
	var memberships []model.ConversationParticipant
	query := s.db.WithContext(ctx).
		Model(&model.ConversationParticipant{}).
		Select("conversation_participants.*").
		Joins("JOIN conversations ON conversations.id = conversation_participants.conversation_id").
		Where("user_id = ?", userID).
		Order("conversations.updated_at DESC, conversations.id DESC").
		Limit(limit + 1)
	if cursor != nil {
		query = query.Where(
			"(conversations.updated_at < (SELECT updated_at FROM conversations WHERE id = ?) OR (conversations.updated_at = (SELECT updated_at FROM conversations WHERE id = ?) AND conversations.id < ?))",
			*cursor,
			*cursor,
			*cursor,
		)
	}
	if err := query.Find(&memberships).Error; err != nil {
		return nil, nil, err
	}

	var nextCursor *int64
	if len(memberships) > limit {
		value := memberships[limit-1].ConversationID
		nextCursor = &value
		memberships = memberships[:limit]
	}
	return memberships, nextCursor, nil
}

func (s *ConversationService) membershipForUser(ctx context.Context, userID, conversationID int64) (model.ConversationParticipant, error) {
	var membership model.ConversationParticipant
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND conversation_id = ?", userID, conversationID).
		First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return membership, ErrConversationForbidden
		}
		return membership, err
	}
	return membership, nil
}

func (s *ConversationService) buildConversationSummary(
	userID int64,
	conversation model.Conversation,
	membership model.ConversationParticipant,
	lastMessage *model.Message,
	unreadCount int64,
) ConversationSummary {
	summary := ConversationSummary{
		ID:          conversation.ID,
		Type:        conversation.Type,
		Title:       conversation.Title,
		UnreadCount: unreadCount,
		IsMuted:     membership.IsMuted,
	}

	if lastMessage != nil {
		summary.LastMessage = lastMessage.Content
		lastMessageAt := lastMessage.CreatedAt
		summary.LastMessageAt = &lastMessageAt
	}

	if summary.Title == "" {
		summary.Title = defaultConversationTitle(conversation.Participants, userID)
	}
	summary.AvatarURL = otherParticipantAvatar(conversation.Participants, userID)

	return summary
}

func (s *ConversationService) loadLatestMessages(ctx context.Context, conversationIDs []int64) (map[int64]*model.Message, error) {
	latest := make(map[int64]*model.Message, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return latest, nil
	}

	var latestIDs []int64
	query := s.db.WithContext(ctx).
		Table("messages AS m").
		Select("m.id").
		Where("m.conversation_id IN ?", conversationIDs).
		Where("m.id = (SELECT m2.id FROM messages AS m2 WHERE m2.conversation_id = m.conversation_id ORDER BY m2.created_at DESC, m2.id DESC LIMIT 1)")
	if err := query.Find(&latestIDs).Error; err != nil {
		return nil, err
	}
	if len(latestIDs) == 0 {
		return latest, nil
	}

	var messages []model.Message
	if err := s.db.WithContext(ctx).
		Preload("Sender.Profile").
		Where("id IN ?", latestIDs).
		Find(&messages).Error; err != nil {
		return nil, err
	}
	for index := range messages {
		message := messages[index]
		latest[message.ConversationID] = &message
	}
	return latest, nil
}

func (s *ConversationService) loadUnreadCounts(ctx context.Context, userID int64, conversationIDs []int64) (map[int64]int64, error) {
	counts := make(map[int64]int64, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return counts, nil
	}

	var rows []struct {
		ConversationID int64 `gorm:"column:conversation_id"`
		Count          int64 `gorm:"column:unread_count"`
	}
	if err := s.db.WithContext(ctx).
		Table("messages AS m").
		Select("m.conversation_id, COUNT(*) AS unread_count").
		Joins("JOIN conversation_participants AS cp ON cp.conversation_id = m.conversation_id AND cp.user_id = ?", userID).
		Where("m.conversation_id IN ? AND m.sender_id <> ?", conversationIDs, userID).
		Where("cp.last_read_at IS NULL OR m.created_at > cp.last_read_at").
		Group("m.conversation_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ConversationID] = row.Count
	}
	return counts, nil
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
