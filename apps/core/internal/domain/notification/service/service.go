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

var ErrNotificationNotFound = errors.New("notification not found")
var ErrNotificationForbidden = errors.New("notification forbidden")

func NewNotificationService(db *gorm.DB) *NotificationService {
	if db == nil {
		db = database.DB
	}
	return &NotificationService{db: db}
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
			var ownedNotification model.Notification
			if lookupErr := s.db.WithContext(ctx).First(&ownedNotification, notificationID).Error; errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return nil, ErrNotificationNotFound
			} else if lookupErr != nil {
				return nil, lookupErr
			}
			return nil, ErrNotificationForbidden
		}
		return nil, err
	}
	if notification.IsRead && notification.ReadAt != nil {
		return &notification, nil
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
