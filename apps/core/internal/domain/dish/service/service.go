package service

import (
	"context"
	"errors"
	"strings"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

var (
	ErrDishNotFound     = errors.New("dish not found")
	ErrDishForbidden    = errors.New("dish forbidden")
	ErrInvalidDishInput = errors.New("invalid dish input")
)

const (
	DishStatusActive   = "active"
	DishStatusDisabled = "disabled"
)

type CreateDishInput struct {
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

func (s *DishService) resolveMerchant(ctx context.Context, userID int64) (*model.Merchant, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDishForbidden
		}
		return nil, err
	}
	return &merchant, nil
}

func (s *DishService) Create(ctx context.Context, userID int64, input CreateDishInput) (*model.Dish, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || input.OriginalPrice < 0 {
		return nil, ErrInvalidDishInput
	}
	merchant, err := s.resolveMerchant(ctx, userID)
	if err != nil {
		return nil, err
	}

	dish := model.Dish{
		MerchantID:    merchant.ID,
		Name:          name,
		ImageURL:      input.ImageURL,
		Description:   input.Description,
		OriginalPrice: input.OriginalPrice,
		Category:      input.Category,
		Status:        DishStatusActive,
	}
	if err := s.db.WithContext(ctx).Create(&dish).Error; err != nil {
		return nil, err
	}
	return &dish, nil
}

func (s *DishService) ListMine(ctx context.Context, userID int64) ([]model.Dish, error) {
	merchant, err := s.resolveMerchant(ctx, userID)
	if err != nil {
		return nil, err
	}
	var dishes []model.Dish
	if err := s.db.WithContext(ctx).
		Where("merchant_id = ?", merchant.ID).
		Order("id desc").
		Find(&dishes).Error; err != nil {
		return nil, err
	}
	return dishes, nil
}

func (s *DishService) loadOwnedDish(ctx context.Context, userID, dishID int64) (*model.Dish, error) {
	merchant, err := s.resolveMerchant(ctx, userID)
	if err != nil {
		return nil, err
	}
	var dish model.Dish
	if err := s.db.WithContext(ctx).First(&dish, dishID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDishNotFound
		}
		return nil, err
	}
	if dish.MerchantID != merchant.ID {
		return nil, ErrDishForbidden
	}
	return &dish, nil
}

func (s *DishService) Update(ctx context.Context, userID, dishID int64, input UpdateDishInput) (*model.Dish, error) {
	dish, err := s.loadOwnedDish(ctx, userID, dishID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" {
			return nil, ErrInvalidDishInput
		}
		updates["name"] = trimmed
	}
	if input.ImageURL != nil {
		updates["image_url"] = *input.ImageURL
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
		updates["category"] = *input.Category
	}
	if len(updates) == 0 {
		return dish, nil
	}
	if err := s.db.WithContext(ctx).Model(&model.Dish{}).Where("id = ?", dishID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.loadOwnedDish(ctx, userID, dishID)
}

func (s *DishService) SetStatus(ctx context.Context, userID, dishID int64, status string) (*model.Dish, error) {
	if status != DishStatusActive && status != DishStatusDisabled {
		return nil, ErrInvalidDishInput
	}
	if _, err := s.loadOwnedDish(ctx, userID, dishID); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.Dish{}).Where("id = ?", dishID).UpdateColumn("status", status).Error; err != nil {
		return nil, err
	}
	return s.loadOwnedDish(ctx, userID, dishID)
}

func (s *DishService) Delete(ctx context.Context, userID, dishID int64) error {
	merchant, err := s.resolveMerchant(ctx, userID)
	if err != nil {
		return err
	}
	var dish model.Dish
	if err := s.db.WithContext(ctx).Unscoped().First(&dish, dishID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDishNotFound
		}
		return err
	}
	if dish.MerchantID != merchant.ID {
		return ErrDishForbidden
	}
	if dish.DeletedAt.Valid {
		return nil
	}
	return s.db.WithContext(ctx).Where("id = ?", dishID).Delete(&model.Dish{}).Error
}
