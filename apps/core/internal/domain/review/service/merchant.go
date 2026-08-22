package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrReviewNotFound  = errors.New("review not found")
	ErrReviewForbidden = errors.New("merchant is not allowed to modify this review")
	ErrInvalidReply    = errors.New("reply text must be between 1 and 500 characters")
)

const reviewStatusActive int16 = 0
const reviewStatusArchived int16 = 1

// ListMerchantReviews returns active reviews belonging to the authenticated
// merchant. The merchant identity is derived from the JWT user id rather than
// from a client-supplied merchant id.
func (s *ReviewService) ListMerchantReviews(ctx context.Context, userID int64) ([]dto.MerchantReview, error) {
	merchant, err := s.merchantForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var reviews []model.Review
	if err := s.db.WithContext(ctx).
		Preload("User.Profile").
		Where("merchant_id = ? AND status = ?", merchant.ID, reviewStatusActive).
		Order("id desc").
		Find(&reviews).Error; err != nil {
		return nil, err
	}

	replies, err := s.merchantReplies(ctx, reviewIDs(reviews))
	if err != nil {
		return nil, err
	}

	items := make([]dto.MerchantReview, 0, len(reviews))
	for _, review := range reviews {
		items = append(items, toMerchantReview(review, replies[review.ID]))
	}
	return items, nil
}

// ReplyToMerchantReview creates or updates the single merchant reply for a
// review. The review owner is checked inside the transaction before writing.
func (s *ReviewService) ReplyToMerchantReview(ctx context.Context, userID, reviewID int64, text string) (dto.MerchantReview, error) {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 500 {
		return dto.MerchantReview{}, ErrInvalidReply
	}

	var review model.Review
	var reply model.ReviewComment
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		merchant, err := merchantForUserTx(tx, userID)
		if err != nil {
			return err
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&review, reviewID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrReviewNotFound
			}
			return err
		}
		if review.Status != reviewStatusActive {
			return ErrReviewNotFound
		}
		if review.MerchantID != merchant.ID {
			return ErrReviewForbidden
		}

		err = tx.Where(
			"review_id = ? AND is_merchant_reply = ? AND parent_comment_id IS NULL AND status = ?",
			review.ID, true, reviewStatusActive,
		).Order("id asc").First(&reply).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			reply = model.ReviewComment{
				ReviewID:        review.ID,
				UserID:          userID,
				Content:         text,
				IsMerchantReply: true,
				Status:          reviewStatusActive,
			}
			if err := tx.Create(&reply).Error; err != nil {
				return err
			}
			if err := tx.Model(&review).UpdateColumn("comment_count", gorm.Expr("comment_count + 1")).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if err := tx.Model(&reply).Updates(map[string]interface{}{
				"content":    text,
				"status":     reviewStatusActive,
				"updated_at": time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
		}
		reply.Content = text
		return nil
	}); err != nil {
		return dto.MerchantReview{}, err
	}

	if err := s.db.WithContext(ctx).Preload("User.Profile").First(&review, review.ID).Error; err != nil {
		return dto.MerchantReview{}, err
	}
	return toMerchantReview(review, &reply), nil
}

// DeleteMerchantReview archives a review after checking merchant ownership.
// The row remains available for audit/history, but is excluded from active
// merchant and public review reads.
func (s *ReviewService) DeleteMerchantReview(ctx context.Context, userID, reviewID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		merchant, err := merchantForUserTx(tx, userID)
		if err != nil {
			return err
		}

		var review model.Review
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&review, reviewID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrReviewNotFound
			}
			return err
		}
		if review.Status != reviewStatusActive {
			return ErrReviewNotFound
		}
		if review.MerchantID != merchant.ID {
			return ErrReviewForbidden
		}

		if err := tx.Model(&review).Update("status", reviewStatusArchived).Error; err != nil {
			return err
		}
		if err := syncMerchantReviewAggregates(tx, review.MerchantID); err != nil {
			return err
		}
		if review.StoreID != nil {
			return syncStoreReviewAggregates(tx, *review.StoreID)
		}
		return nil
	})
}

func (s *ReviewService) merchantForUser(ctx context.Context, userID int64) (*model.Merchant, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMerchantNotFound
		}
		return nil, err
	}
	return &merchant, nil
}

func merchantForUserTx(tx *gorm.DB, userID int64) (*model.Merchant, error) {
	var merchant model.Merchant
	if err := tx.Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMerchantNotFound
		}
		return nil, err
	}
	return &merchant, nil
}

func (s *ReviewService) merchantReplies(ctx context.Context, ids []int64) (map[int64]*model.ReviewComment, error) {
	result := make(map[int64]*model.ReviewComment)
	if len(ids) == 0 {
		return result, nil
	}

	var replies []model.ReviewComment
	if err := s.db.WithContext(ctx).
		Where("review_id IN ? AND is_merchant_reply = ? AND parent_comment_id IS NULL AND status = ?", ids, true, reviewStatusActive).
		Order("id asc").
		Find(&replies).Error; err != nil {
		return nil, err
	}
	for i := range replies {
		if _, exists := result[replies[i].ReviewID]; !exists {
			result[replies[i].ReviewID] = &replies[i]
		}
	}
	return result, nil
}

func reviewIDs(reviews []model.Review) []int64 {
	ids := make([]int64, 0, len(reviews))
	for _, review := range reviews {
		ids = append(ids, review.ID)
	}
	return ids
}

func toMerchantReview(review model.Review, reply *model.ReviewComment) dto.MerchantReview {
	customerName := fmt.Sprintf("Customer %d", review.UserID)
	if review.User != nil && review.User.Profile != nil && strings.TrimSpace(review.User.Profile.Nickname) != "" {
		customerName = review.User.Profile.Nickname
	}

	item := dto.MerchantReview{
		ID:           review.ID,
		CustomerName: customerName,
		Rating:       review.Rating,
		Text:         review.Content,
		Date:         review.CreatedAt.UTC().Format(time.RFC3339),
	}
	if reply != nil {
		item.HasReply = true
		item.ReplyText = reply.Content
	}
	return item
}
