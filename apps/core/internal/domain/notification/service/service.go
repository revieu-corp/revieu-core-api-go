package service

import (
	"context"
	"errors"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

type NotificationService struct {
	db *gorm.DB
}

const (
	DefaultNotificationListLimit = 20
	MaxNotificationListLimit     = 100
)

// NotificationListQuery controls the authenticated notification list.
// Cursor is the last notification id returned by the previous page.
type NotificationListQuery struct {
	Limit  int
	Cursor *int64
}

var ErrNotificationNotFound = errors.New("notification not found")

func NewNotificationService(db *gorm.DB) *NotificationService {
	if db == nil {
		db = database.DB
	}
	return &NotificationService{db: db}
}

func (s *NotificationService) List(ctx context.Context, userID int64, query NotificationListQuery) ([]model.Notification, *int64, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = DefaultNotificationListLimit
	}
	if limit > MaxNotificationListLimit {
		limit = MaxNotificationListLimit
	}

	notifications := make([]model.Notification, 0, limit)
	dbQuery := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc, id desc").
		Limit(limit + 1)
	if query.Cursor != nil {
		dbQuery = dbQuery.Where("id < ?", *query.Cursor)
	}
	if err := dbQuery.Find(&notifications).Error; err != nil {
		return nil, nil, err
	}

	var nextCursor *int64
	if len(notifications) > limit {
		value := notifications[limit-1].ID
		nextCursor = &value
		notifications = notifications[:limit]
	}
	return notifications, nextCursor, nil
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
