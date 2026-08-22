package service

import (
	"context"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

type ContentService struct {
	db *gorm.DB
}

func NewContentService(db *gorm.DB) *ContentService {
	if db == nil {
		db = database.DB
	}
	return &ContentService{db: db}
}

func (s *ContentService) ListUserPosts(ctx context.Context, userID int64, cursor *int64, limit int) ([]model.Post, int64, *int64, error) {
	baseQuery := s.db.WithContext(ctx).Model(&model.Post{}).Where("user_id = ?", userID)
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, nil, err
	}
	q := s.db.WithContext(ctx).Model(&model.Post{}).Where("user_id = ?", userID)
	if cursor != nil {
		q = q.Where("id < ?", *cursor)
	}
	var posts []model.Post
	if err := q.Order("id desc").Limit(limit + 1).Find(&posts).Error; err != nil {
		return nil, 0, nil, err
	}
	var nextCursor *int64
	if len(posts) > limit {
		cursorValue := posts[limit-1].ID
		nextCursor = &cursorValue
		posts = posts[:limit]
	}
	return posts, total, nextCursor, nil
}

func (s *ContentService) ListUserReviews(ctx context.Context, userID int64, cursor *int64, limit int) ([]model.Review, int64, *int64, error) {
	baseQuery := s.db.WithContext(ctx).Model(&model.Review{}).Where("user_id = ?", userID)
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, nil, err
	}

	q := baseQuery.Order("id desc")
	if cursor != nil {
		q = q.Where("id < ?", *cursor)
	}
	var reviews []model.Review
	if err := q.Limit(limit + 1).Find(&reviews).Error; err != nil {
		return nil, 0, nil, err
	}
	var nextCursor *int64
	if len(reviews) > limit {
		cursorValue := reviews[limit-1].ID
		nextCursor = &cursorValue
		reviews = reviews[:limit]
	}
	return reviews, total, nextCursor, nil
}

func (s *ContentService) ListFavorites(ctx context.Context, userID int64, targetType string, cursor *int64, limit int) ([]model.Favorite, int64, *int64, error) {
	baseQuery := s.db.WithContext(ctx).Model(&model.Favorite{}).Where("user_id = ?", userID)
	if targetType != "" {
		baseQuery = baseQuery.Where("target_type = ?", targetType)
	}
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, nil, err
	}
	q := s.db.WithContext(ctx).Model(&model.Favorite{}).Where("user_id = ?", userID)
	if targetType != "" {
		q = q.Where("target_type = ?", targetType)
	}
	if cursor != nil {
		q = q.Where("id < ?", *cursor)
	}
	var items []model.Favorite
	if err := q.Order("id desc").Limit(limit + 1).Find(&items).Error; err != nil {
		return nil, 0, nil, err
	}
	var nextCursor *int64
	if len(items) > limit {
		cursorValue := items[limit-1].ID
		nextCursor = &cursorValue
		items = items[:limit]
	}
	return items, total, nextCursor, nil
}

func (s *ContentService) ListLikes(ctx context.Context, userID int64, cursor *int64, limit int) ([]model.Like, int64, *int64, error) {
	baseQuery := s.db.WithContext(ctx).Model(&model.Like{}).Where("user_id = ?", userID)
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, nil, err
	}
	q := s.db.WithContext(ctx).Model(&model.Like{}).Where("user_id = ?", userID)
	if cursor != nil {
		q = q.Where("id < ?", *cursor)
	}
	var items []model.Like
	if err := q.Order("id desc").Limit(limit + 1).Find(&items).Error; err != nil {
		return nil, 0, nil, err
	}
	var nextCursor *int64
	if len(items) > limit {
		cursorValue := items[limit-1].ID
		nextCursor = &cursorValue
		items = items[:limit]
	}
	return items, total, nextCursor, nil
}
