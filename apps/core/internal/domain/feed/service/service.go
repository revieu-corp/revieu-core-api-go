package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/feed/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

type FeedService struct {
	db *gorm.DB
}

func NewFeedService(db *gorm.DB) *FeedService {
	if db == nil {
		db = database.DB
	}
	return &FeedService{db: db}
}

var ErrInvalidCursor = errors.New("invalid feed cursor")

type feedCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
}

type candidate struct {
	item      dto.FeedItem
	createdAt time.Time
	id        int64
	typeName  string
}

func (s *FeedService) Home(ctx context.Context, viewerID int64, cursor string, limit int) ([]dto.FeedItem, *string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	parsedCursor, err := decodeCursor(cursor)
	if err != nil {
		return nil, nil, err
	}

	var posts []model.Post
	postQuery := s.db.WithContext(ctx).
		Model(&model.Post{}).
		Where("posts.status = ?", 0)
	postQuery = applyUserVisibility(postQuery, "posts.user_id", viewerID)
	postQuery = applyCursor(postQuery, "post", parsedCursor)
	if err := postQuery.Order("posts.created_at desc, posts.id desc").Limit(limit + 1).Find(&posts).Error; err != nil {
		return nil, nil, err
	}

	var reviews []model.Review
	reviewQuery := s.db.WithContext(ctx).
		Model(&model.Review{}).
		Where("reviews.status = ?", 0)
	reviewQuery = applyUserVisibility(reviewQuery, "reviews.user_id", viewerID)
	reviewQuery = applyCursor(reviewQuery, "review", parsedCursor)
	if err := reviewQuery.Order("reviews.created_at desc, reviews.id desc").Limit(limit + 1).Find(&reviews).Error; err != nil {
		return nil, nil, err
	}

	var merchants []model.Merchant
	merchantQuery := s.db.WithContext(ctx).
		Model(&model.Merchant{}).
		Where("status = ? AND verification_status = ?", 0, "verified")
	merchantQuery = applyCursor(merchantQuery, "merchant", parsedCursor)
	if err := merchantQuery.Order("created_at desc, id desc").Limit(limit + 1).Find(&merchants).Error; err != nil {
		return nil, nil, err
	}

	candidates := make([]candidate, 0, len(posts)+len(reviews)+len(merchants))
	for _, post := range posts {
		title := post.Title
		if title == "" {
			title = post.Content
		}
		candidates = append(candidates, candidate{
			item: dto.FeedItem{
				ID:        fmt.Sprintf("post:%d", post.ID),
				Type:      "post",
				Title:     title,
				Image:     firstImage(post.Images),
				CreatedAt: post.CreatedAt,
			},
			createdAt: post.CreatedAt,
			id:        post.ID,
			typeName:  "post",
		})
	}
	for _, review := range reviews {
		candidates = append(candidates, candidate{
			item: dto.FeedItem{
				ID:        fmt.Sprintf("review:%d", review.ID),
				Type:      "review",
				Title:     review.Content,
				Image:     firstImage(review.Images),
				CreatedAt: review.CreatedAt,
			},
			createdAt: review.CreatedAt,
			id:        review.ID,
			typeName:  "review",
		})
	}
	for _, merchant := range merchants {
		image := merchant.CoverImage
		if image == "" {
			image = merchant.LogoURL
		}
		candidates = append(candidates, candidate{
			item: dto.FeedItem{
				ID:        fmt.Sprintf("merchant:%d", merchant.ID),
				Type:      "merchant",
				Title:     merchant.Name,
				Image:     image,
				CreatedAt: merchant.CreatedAt,
			},
			createdAt: merchant.CreatedAt,
			id:        merchant.ID,
			typeName:  "merchant",
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].createdAt.Equal(candidates[j].createdAt) {
			return candidates[i].createdAt.After(candidates[j].createdAt)
		}
		if candidates[i].typeName != candidates[j].typeName {
			return candidates[i].typeName > candidates[j].typeName
		}
		return candidates[i].id > candidates[j].id
	})

	var next *string
	if len(candidates) > limit {
		last := candidates[limit-1]
		encoded := encodeCursor(feedCursor{CreatedAt: last.createdAt, ID: last.id, Type: last.typeName})
		next = &encoded
		candidates = candidates[:limit]
	}
	items := make([]dto.FeedItem, 0, len(candidates))
	for _, item := range candidates {
		items = append(items, item.item)
	}
	return items, next, nil
}

func applyCursor(q *gorm.DB, typeName string, cursor *feedCursor) *gorm.DB {
	if cursor == nil {
		return q
	}
	return q.Where(
		`created_at < ? OR (created_at = ? AND ? < ?) OR (created_at = ? AND ? = ? AND id < ?)`,
		cursor.CreatedAt,
		cursor.CreatedAt,
		typeName,
		cursor.Type,
		cursor.CreatedAt,
		typeName,
		cursor.Type,
		cursor.ID,
	)
}

func applyUserVisibility(q *gorm.DB, ownerColumn string, viewerID int64) *gorm.DB {
	condition := fmt.Sprintf(`(
		NOT EXISTS (
			SELECT 1 FROM user_privacies privacy
			WHERE privacy.user_id = %s
		)
		OR EXISTS (
			SELECT 1 FROM user_privacies privacy
			WHERE privacy.user_id = %s AND privacy.is_public = ?
		)
		OR %s = ?
		OR EXISTS (
			SELECT 1 FROM user_follows follow
			WHERE follow.following_id = %s AND follow.follower_id = ?
		)
	)`, ownerColumn, ownerColumn, ownerColumn, ownerColumn)
	return q.Where(condition, true, viewerID, viewerID)
}

func encodeCursor(cursor feedCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(raw string) (*feedCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var cursor feedCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.ID <= 0 || cursor.CreatedAt.IsZero() || cursor.Type == "" {
		return nil, ErrInvalidCursor
	}
	return &cursor, nil
}

func firstImage(raw string) string {
	var images []string
	if err := json.Unmarshal([]byte(raw), &images); err != nil || len(images) == 0 {
		return ""
	}
	return images[0]
}
