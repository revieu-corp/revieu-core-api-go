package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReviewService struct {
	db *gorm.DB
}

var ErrMerchantNotFound = errors.New("merchant not found")
var ErrStoreNotFound = errors.New("store not found")
var ErrStoreMerchantMismatch = errors.New("store does not belong to merchant")

func NewReviewService(db *gorm.DB) *ReviewService {
	if db == nil {
		db = database.DB
	}
	return &ReviewService{db: db}
}

func (s *ReviewService) Detail(ctx context.Context, id int64) (*model.Review, error) {
	var review model.Review
	if err := s.db.WithContext(ctx).Preload("Merchant").Preload("Store").First(&review, id).Error; err != nil {
		return nil, err
	}
	if review.Status != reviewStatusActive {
		return nil, gorm.ErrRecordNotFound
	}
	return &review, nil
}

func (s *ReviewService) Create(ctx context.Context, userID int64, req dto.Review) (model.Review, error) {
	merchantID, err := req.MerchantIDValue()
	if err != nil {
		return model.Review{}, err
	}

	venueID := merchantID
	if req.VenueID != "" {
		venueID, err = req.VenueIDValue()
		if err != nil {
			return model.Review{}, err
		}
	}
	storeID, err := req.StoreIDValue()
	if err != nil {
		return model.Review{}, err
	}

	visitDate, err := req.VisitDateValue()
	if err != nil {
		return model.Review{}, err
	}

	imagesJSON, _ := json.Marshal(req.Images)
	review := model.Review{
		UserID:     userID,
		MerchantID: merchantID,
		VenueID:    venueID,
		StoreID:    storeID,
		Rating:     float32(req.Rating),
		Content:    req.Text,
		Images:     string(imagesJSON),
		VisitDate:  visitDate,
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var merchant model.Merchant
		if err := tx.Select("id").First(&merchant, merchantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMerchantNotFound
			}
			return err
		}

		if storeID != nil {
			var store model.Store
			if err := tx.Select("id", "merchant_id").First(&store, *storeID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrStoreNotFound
				}
				return err
			}
			if store.MerchantID != merchantID {
				return ErrStoreMerchantMismatch
			}
		}

		if err := tx.Create(&review).Error; err != nil {
			return err
		}
		if err := syncMerchantReviewAggregates(tx, merchantID); err != nil {
			return err
		}
		if storeID != nil {
			if err := syncStoreReviewAggregates(tx, *storeID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return model.Review{}, err
	}

	return review, nil
}

func (s *ReviewService) Like(ctx context.Context, userID, reviewID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var review model.Review
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&review, reviewID).Error; err != nil {
			return err
		}

		var existing model.Like
		if err := tx.Where("user_id = ? AND target_type = ? AND target_id = ?", userID, "review", reviewID).
			First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		like := model.Like{UserID: userID, TargetType: "review", TargetID: reviewID}
		if err := tx.Create(&like).Error; err != nil {
			// Concurrent duplicate like should remain idempotent.
			if errors.Is(err, gorm.ErrDuplicatedKey) || isUniqueViolation(err) {
				return nil
			}
			return err
		}

		return tx.Model(&review).UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error
	})
}

func (s *ReviewService) Comment(ctx context.Context, userID, reviewID int64, text string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var review model.Review
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&review, reviewID).Error; err != nil {
			return err
		}
		comment := model.ReviewComment{ReviewID: reviewID, UserID: userID, Content: text}
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		return tx.Model(&review).UpdateColumn("comment_count", gorm.Expr("comment_count + 1")).Error
	})
}

func syncMerchantReviewAggregates(tx *gorm.DB, merchantID int64) error {
	type aggregate struct {
		Count int64
		Avg   float64
	}
	var agg aggregate
	if err := tx.Model(&model.Review{}).
		Select("COUNT(*) AS count, COALESCE(AVG(rating), 0) AS avg").
		Where("merchant_id = ? AND status = ?", merchantID, reviewStatusActive).
		Scan(&agg).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"review_count":  int(agg.Count),
		"total_reviews": int(agg.Count),
		"avg_rating":    float32(agg.Avg),
	}
	return tx.Model(&model.Merchant{}).Where("id = ?", merchantID).Updates(updates).Error
}

func syncStoreReviewAggregates(tx *gorm.DB, storeID int64) error {
	type aggregate struct {
		Count int64
		Avg   float64
	}
	var agg aggregate
	if err := tx.Model(&model.Review{}).
		Select("COUNT(*) AS count, COALESCE(AVG(rating), 0) AS avg").
		Where("store_id = ? AND status = ?", storeID, reviewStatusActive).
		Scan(&agg).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{
		"review_count": int(agg.Count),
		"avg_rating":   float32(agg.Avg),
	}
	return tx.Model(&model.Store{}).Where("id = ?", storeID).Updates(updates).Error
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "UNIQUE constraint failed")
}
