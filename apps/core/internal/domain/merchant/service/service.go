package service

import (
	"context"
	"errors"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

type MerchantService struct {
	db *gorm.DB
}

var ErrMerchantNotFound = errors.New("merchant not found")

func NewMerchantService(db *gorm.DB) *MerchantService {
	if db == nil {
		db = database.DB
	}
	return &MerchantService{db: db}
}

func (s *MerchantService) List(ctx context.Context, category, search string) ([]model.Merchant, error) {
	q := s.publicScope(s.db.WithContext(ctx).Model(&model.Merchant{}))
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if search != "" {
		pattern := "%" + search + "%"
		q = q.Where("name ILIKE ? OR business_name ILIKE ?", pattern, pattern)
	}
	var merchants []model.Merchant
	if err := q.Order("id desc").Find(&merchants).Error; err != nil {
		return nil, err
	}
	return merchants, nil
}

func (s *MerchantService) Detail(ctx context.Context, id int64) (*model.Merchant, error) {
	var merchant model.Merchant
	if err := s.publicScope(s.db.WithContext(ctx)).First(&merchant, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMerchantNotFound
		}
		return nil, err
	}
	return &merchant, nil
}

func (s *MerchantService) Reviews(ctx context.Context, merchantID int64) ([]model.Review, error) {
	if _, err := s.Detail(ctx, merchantID); err != nil {
		return nil, err
	}
	var reviews []model.Review
	if err := s.db.WithContext(ctx).Where("merchant_id = ? AND status = ?", merchantID, 0).Order("id desc").Find(&reviews).Error; err != nil {
		return nil, err
	}
	return reviews, nil
}

func (s *MerchantService) publicScope(q *gorm.DB) *gorm.DB {
	return q.Where("status = ? AND verification_status = ?", 0, "verified")
}
