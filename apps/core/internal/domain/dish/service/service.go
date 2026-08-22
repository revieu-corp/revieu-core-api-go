package service

import (
	"context"
	"errors"
	"strings"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

const (
	DishStatusActive   = "active"
	DishStatusDisabled = "disabled"
)

var (
	ErrDishNotFound     = errors.New("dish not found")
	ErrDishForbidden    = errors.New("dish forbidden")
	ErrInvalidDishInput = errors.New("invalid dish input")
	ErrMerchantNotFound = errors.New("merchant not found")
)

type UpsertDishInput struct {
	Name          string
	ImageURL      string
	Description   string
	OriginalPrice float64
	Category      string
}

type UpdateDishInput struct {
	Name          *string
	ImageURL      *string
	Description   *string
	OriginalPrice *float64
	Category      *string
}

type DishService struct {
	db *gorm.DB
}

func NewDishService(db *gorm.DB) *DishService {
	if db == nil {
		db = database.DB
	}
	return &DishService{db: db}
}

func (s *DishService) ListMine(ctx context.Context, userID int64) ([]model.Dish, error) {
	merchantID, err := s.merchantID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var dishes []model.Dish
	if err := s.db.WithContext(ctx).
		Where("merchant_id = ?", merchantID).
		Order("id DESC").
		Find(&dishes).Error; err != nil {
		return nil, err
	}
	return dishes, nil
}

func (s *DishService) Create(ctx context.Context, userID int64, input UpsertDishInput) (*model.Dish, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || input.OriginalPrice < 0 {
		return nil, ErrInvalidDishInput
	}
	merchantID, err := s.merchantID(ctx, userID)
	if err != nil {
		return nil, err
	}

	dish := model.Dish{
		MerchantID:    merchantID,
		Name:          name,
		ImageURL:      strings.TrimSpace(input.ImageURL),
		Description:   input.Description,
		OriginalPrice: input.OriginalPrice,
		Category:      strings.TrimSpace(input.Category),
		Status:        DishStatusActive,
	}
	if err := s.db.WithContext(ctx).Create(&dish).Error; err != nil {
		return nil, err
	}
	return &dish, nil
}

func (s *DishService) Update(ctx context.Context, userID, dishID int64, input UpdateDishInput) (*model.Dish, error) {
	dish, err := s.ownedDish(ctx, userID, dishID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, ErrInvalidDishInput
		}
		updates["name"] = name
	}
	if input.ImageURL != nil {
		updates["image_url"] = strings.TrimSpace(*input.ImageURL)
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.OriginalPrice != nil {
		if *input.OriginalPrice < 0 {
			return nil, ErrInvalidDishInput
		}
		updates["original_price"] = *input.OriginalPrice
	}
	if input.Category != nil {
		updates["category"] = strings.TrimSpace(*input.Category)
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&model.Dish{}).Where("id = ?", dish.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	var updated model.Dish
	if err := s.db.WithContext(ctx).First(&updated, dish.ID).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *DishService) Delete(ctx context.Context, userID, dishID int64) error {
	dish, err := s.ownedDish(ctx, userID, dishID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&model.Dish{}, dish.ID).Error
}

func (s *DishService) SetStatus(ctx context.Context, userID, dishID int64, status string) (*model.Dish, error) {
	if status != DishStatusActive && status != DishStatusDisabled {
		return nil, ErrInvalidDishInput
	}
	dish, err := s.ownedDish(ctx, userID, dishID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.Dish{}).Where("id = ?", dish.ID).Update("status", status).Error; err != nil {
		return nil, err
	}
	dish.Status = status
	return dish, nil
}

func (s *DishService) merchantID(ctx context.Context, userID int64) (int64, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrMerchantNotFound
		}
		return 0, err
	}
	return merchant.ID, nil
}

func (s *DishService) ownedDish(ctx context.Context, userID, dishID int64) (*model.Dish, error) {
	merchantID, err := s.merchantID(ctx, userID)
	if err != nil {
		return nil, err
	}
	var dish model.Dish
	if err := s.db.WithContext(ctx).Where("id = ? AND merchant_id = ?", dishID, merchantID).First(&dish).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDishNotFound
		}
		return nil, err
	}
	return &dish, nil
}
