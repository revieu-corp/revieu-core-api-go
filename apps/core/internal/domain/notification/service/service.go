package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationService struct {
	db *gorm.DB
}

var (
	ErrNotificationNotFound = errors.New("notification not found")
	ErrNotificationInvalid  = errors.New("invalid notification event")
)

// EventInput is the stable internal contract for domain notification
// producers. Data is persisted as JSON for consumers of the notification API.
type EventInput struct {
	RecipientID int64
	Type        string
	Title       string
	Content     string
	Data        map[string]interface{}
	DedupKey    string
}

var notificationTypes = map[string]struct{}{
	model.NotificationTypeOrderPaid:                   {},
	model.NotificationTypeVoucherRedeemed:             {},
	model.NotificationTypeMerchantVerificationChanged: {},
	model.NotificationTypeReviewLiked:                 {},
	model.NotificationTypeReviewCommented:             {},
}

func NewNotificationService(db *gorm.DB) *NotificationService {
	if db == nil {
		db = database.DB
	}
	return &NotificationService{db: db}
}

// CreateEvent creates a notification for a supported domain event. A
// non-empty DedupKey makes retries idempotent and is backed by a partial
// unique index in the PostgreSQL migration.
func (s *NotificationService) CreateEvent(ctx context.Context, input EventInput) (*model.Notification, bool, error) {
	return CreateEventTx(ctx, s.db, input)
}

// CreateEventTx writes an event using the supplied database handle. Callers
// can pass their current transaction so the business mutation and its
// notification commit or roll back together.
func CreateEventTx(ctx context.Context, db *gorm.DB, input EventInput) (*model.Notification, bool, error) {
	if db == nil || input.RecipientID <= 0 {
		return nil, false, ErrNotificationInvalid
	}
	typeName := strings.TrimSpace(input.Type)
	if _, ok := notificationTypes[typeName]; !ok || strings.TrimSpace(input.Title) == "" {
		return nil, false, ErrNotificationInvalid
	}

	dedupKey := strings.TrimSpace(input.DedupKey)
	if dedupKey != "" {
		var existing model.Notification
		if err := db.WithContext(ctx).Where("dedup_key = ?", dedupKey).First(&existing).Error; err == nil {
			return &existing, false, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
	}

	payload := "{}"
	if input.Data != nil {
		encoded, err := json.Marshal(input.Data)
		if err != nil {
			return nil, false, fmt.Errorf("marshal notification data: %w", err)
		}
		payload = string(encoded)
	}

	var dedup *string
	if dedupKey != "" {
		dedup = &dedupKey
	}
	notification := model.Notification{
		UserID:   input.RecipientID,
		Type:     typeName,
		Title:    strings.TrimSpace(input.Title),
		Content:  strings.TrimSpace(input.Content),
		Data:     payload,
		DedupKey: dedup,
		IsRead:   false,
	}
	result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&notification)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 && dedup != nil {
		var existing model.Notification
		if err := db.WithContext(ctx).Where("dedup_key = ?", dedupKey).First(&existing).Error; err != nil {
			return nil, false, err
		}
		return &existing, false, nil
	}
	return &notification, true, nil
}

func (s *NotificationService) List(ctx context.Context, userID int64) ([]model.Notification, error) {
	var notifications []model.Notification
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("is_read asc, created_at desc").
		Find(&notifications).Error; err != nil {
		return nil, err
	}
	return notifications, nil
}

func (s *NotificationService) MarkRead(ctx context.Context, userID, notificationID int64) (*model.Notification, error) {
	var notification model.Notification
	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", notificationID, userID).
		First(&notification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotificationNotFound
		}
		return nil, err
	}

	now := time.Now().UTC()
	notification.IsRead = true
	notification.ReadAt = &now
	if err := s.db.WithContext(ctx).Save(&notification).Error; err != nil {
		return nil, err
	}
	return &notification, nil
}

func (s *NotificationService) ReadAll(ctx context.Context, userID int64) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).
		Model(&model.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": &now,
		}).Error
}
