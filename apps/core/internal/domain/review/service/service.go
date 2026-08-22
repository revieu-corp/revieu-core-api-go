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

const (
	minReviewRating = 1.0
	maxReviewRating = 5.0
	maxTagLength    = 50
)

func NewReviewService(db *gorm.DB) *ReviewService {
	if db == nil {
		db = database.DB
	}
	return &ReviewService{db: db}
}

func (s *ReviewService) Detail(ctx context.Context, id int64) (*model.Review, error) {
	var review model.Review
	if err := s.db.WithContext(ctx).
		Preload("Merchant").
		Preload("Store").
		Preload("Tags").
		First(&review, id).Error; err != nil {
		return nil, err
	}
	return &review, nil
}

func (s *ReviewService) Create(ctx context.Context, userID int64, req dto.Review) (model.Review, error) {
	if err := validateReviewRatings(req); err != nil {
		return model.Review{}, err
	}

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
		UserID:           userID,
		MerchantID:       merchantID,
		VenueID:          venueID,
		StoreID:          storeID,
		Rating:           float32(req.Rating),
		RatingEnv:        float64ToFloat32(req.RatingEnv),
		RatingService:    float64ToFloat32(req.RatingService),
		RatingValue:      float64ToFloat32(req.RatingValue),
		RatingFood:       float64ToFloat32(req.RatingFood),
		LocationVerified: req.LocationVerified,
		Content:          req.Text,
		Images:           string(imagesJSON),
		VisitDate:        visitDate,
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
		if err := replaceReviewTags(tx, &review, req.Tags); err != nil {
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

func validateReviewRatings(req dto.Review) error {
	if err := validateRating("rating", req.Rating); err != nil {
		return err
	}
	for name, value := range map[string]*float64{
		"ratingEnv":     req.RatingEnv,
		"ratingService": req.RatingService,
		"ratingValue":   req.RatingValue,
		"ratingFood":    req.RatingFood,
	} {
		if value != nil {
			if err := validateRating(name, *value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRating(name string, value float64) error {
	if value < minReviewRating || value > maxReviewRating {
		return errors.New(name + " must be between 1 and 5")
	}
	return nil
}

func float64ToFloat32(value *float64) *float32 {
	if value == nil {
		return nil
	}
	converted := float32(*value)
	return &converted
}

func replaceReviewTags(tx *gorm.DB, review *model.Review, names []string) error {
	normalized := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if len([]rune(name)) > maxTagLength {
			return errors.New("tag must be at most 50 characters")
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}

	tags := make([]model.Tag, 0, len(normalized))
	for _, name := range normalized {
		var tag model.Tag
		if err := tx.Where("name = ?", name).FirstOrCreate(&tag, model.Tag{Name: name}).Error; err != nil {
			return err
		}
		tags = append(tags, tag)
	}

	review.Tags = tags
	return tx.Model(review).Association("Tags").Replace(tags)
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
		Where("merchant_id = ?", merchantID).
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
		Where("store_id = ?", storeID).
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
