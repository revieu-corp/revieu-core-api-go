package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

const (
	storeStatusPublished  int16 = 1
	couponStatusActive          = "active"
	couponStatusDraft           = "draft"
	couponStatusDisabled        = "disabled"
	couponStatusSoldOut         = "sold_out"
	couponStatusExpired         = "expired"
	couponStatusScheduled       = "scheduled"
)

var (
	ErrCouponNotFound          = errors.New("coupon not found")
	ErrCouponInactive          = errors.New("coupon inactive")
	ErrCouponExpired           = errors.New("coupon expired")
	ErrCouponNotStarted        = errors.New("coupon not started")
	ErrCouponSoldOut           = errors.New("coupon sold out")
	ErrCouponNotStoreScoped    = errors.New("coupon must be store scoped")
	ErrCouponStoreMismatch     = errors.New("coupon store mismatch")
	ErrCouponPerUserLimit      = errors.New("coupon per-user limit exceeded")
	ErrStoreNotFound           = errors.New("store not found")
	ErrStoreNotPublished       = errors.New("store not published")
	ErrStoreForbidden          = errors.New("store forbidden")
	ErrInvalidCouponInput      = errors.New("invalid coupon input")
	ErrDeprecatedCouponRedeem  = errors.New("coupon direct redeem is deprecated, redeem voucher instead")
	ErrDeprecatedCouponPayment = errors.New("coupon payment initiation is deprecated, use order payment")
)

type CreateStoreCouponInput struct {
	Title              string
	Description        string
	Type               string
	CouponType         string
	Price              float64
	OriginalPrice      float64
	SalePrice          float64
	DiscountPercentage float64
	ImageURL           string
	DishIDs            []int64
	TotalQuantity      int
	MaxPerUser         int
	ValidFrom          *time.Time
	ValidUntil         *time.Time
	Terms              string
	Status             string
}

type UpdateStoreCouponInput struct {
	Title              *string
	Description        *string
	CouponType         *string
	ImageURL           *string
	Price              *float64
	OriginalPrice      *float64
	SalePrice          *float64
	DiscountPercentage *float64
	DishIDs            *[]int64
	TotalQuantity      *int
	MaxPerUser         *int
	ValidFrom          **time.Time
	ValidUntil         **time.Time
	Terms              *string
	Status             *string
}

type ValidateInput struct {
	Quantity int
	UserID   *int64
}

type ValidateResult struct {
	CouponID    int64   `json:"coupon_id"`
	StoreID     int64   `json:"store_id"`
	MerchantID  int64   `json:"merchant_id"`
	Quantity    int     `json:"quantity"`
	Remaining   int     `json:"remaining"`
	Price       float64 `json:"price"`
	MaxPerUser  int     `json:"max_per_user"`
	IsValid     bool    `json:"is_valid"`
	Status      string  `json:"status"`
	Description string  `json:"description"`
}

type CouponService struct {
	db *gorm.DB
}

func NewCouponService(db *gorm.DB) *CouponService {
	if db == nil {
		db = database.DB
	}
	return &CouponService{db: db}
}

// ComputeStatus derives the effective, display-facing status for a coupon.
// draft/disabled are terminal, merchant-controlled states and are never
// overridden by quantity or date checks. now is passed in explicitly so
// this stays a pure, deterministically testable function. Exported because
// the handler package uses it to compute the status shown in API responses
// without persisting it (see Task 9).
func ComputeStatus(coupon model.Coupon, now time.Time) string {
	if coupon.Status == couponStatusDraft || coupon.Status == couponStatusDisabled {
		return coupon.Status
	}
	if coupon.TotalQuantity > 0 && coupon.ClaimedCount >= coupon.TotalQuantity {
		return couponStatusSoldOut
	}
	if coupon.ValidUntil != nil && coupon.ValidUntil.Before(now) {
		return couponStatusExpired
	}
	if coupon.ValidFrom != nil && coupon.ValidFrom.After(now) {
		return couponStatusScheduled
	}
	return coupon.Status
}

func (s *CouponService) CreateForStore(ctx context.Context, userID, storeID int64, input CreateStoreCouponInput) (*model.Coupon, error) {
	title := strings.TrimSpace(input.Title)
	couponType := strings.TrimSpace(input.Type)
	if title == "" || couponType == "" || input.TotalQuantity <= 0 {
		return nil, ErrInvalidCouponInput
	}
	if input.MaxPerUser <= 0 {
		return nil, ErrInvalidCouponInput
	}
	if err := validateCouponPricing(input.Price, input.OriginalPrice, input.SalePrice, input.DiscountPercentage); err != nil {
		return nil, err
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && input.ValidFrom.After(*input.ValidUntil) {
		return nil, ErrInvalidCouponInput
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = couponStatusActive
	}
	if !isMerchantControlledCouponStatus(status) {
		return nil, ErrInvalidCouponInput
	}

	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreForbidden
		}
		return nil, err
	}

	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}
	if store.MerchantID != merchant.ID {
		return nil, ErrStoreForbidden
	}
	if store.Status != storeStatusPublished {
		return nil, ErrStoreNotPublished
	}
	if err := s.validateDishIDs(ctx, merchant.ID, input.DishIDs); err != nil {
		return nil, err
	}
	if input.Price == 0 && input.SalePrice > 0 {
		// Price is the legacy mirror of sale_price. Keep old callers that only
		// send sale_price compatible while storing one consistent value.
		input.Price = input.SalePrice
	}

	imageURL := strings.TrimSpace(input.ImageURL)
	if imageURL == "" && len(input.DishIDs) == 1 {
		var dish model.Dish
		if err := s.db.WithContext(ctx).Where("id = ? AND merchant_id = ?", input.DishIDs[0], merchant.ID).First(&dish).Error; err == nil {
			imageURL = dish.ImageURL
		}
	}
	dishIDs := input.DishIDs
	if dishIDs == nil {
		dishIDs = []int64{}
	}
	dishIDsJSON, err := json.Marshal(dishIDs)
	if err != nil {
		return nil, err
	}

	coupon := model.Coupon{
		MerchantID:         store.MerchantID,
		StoreID:            &store.ID,
		Title:              title,
		Description:        input.Description,
		Type:               couponType,
		CouponType:         input.CouponType,
		Price:              input.Price,
		OriginalPrice:      input.OriginalPrice,
		SalePrice:          input.SalePrice,
		DiscountPercentage: input.DiscountPercentage,
		ImageURL:           imageURL,
		DishIDs:            string(dishIDsJSON),
		TotalQuantity:      input.TotalQuantity,
		MaxPerUser:         input.MaxPerUser,
		Terms:              input.Terms,
		Status:             status,
	}
	if input.ValidFrom != nil {
		validFrom := *input.ValidFrom
		coupon.ValidFrom = &validFrom
	}
	if input.ValidUntil != nil {
		validUntil := *input.ValidUntil
		coupon.ValidUntil = &validUntil
		coupon.ExpiryDate = validUntil
	}

	if err := s.db.WithContext(ctx).Create(&coupon).Error; err != nil {
		return nil, err
	}
	return &coupon, nil
}

func (s *CouponService) DeleteForStore(ctx context.Context, userID, storeID, couponID int64) error {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStoreForbidden
		}
		return err
	}

	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStoreNotFound
		}
		return err
	}
	if store.MerchantID != merchant.ID {
		return ErrStoreForbidden
	}

	var coupon model.Coupon
	if err := s.db.WithContext(ctx).Unscoped().First(&coupon, couponID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCouponNotFound
		}
		return err
	}
	if coupon.StoreID == nil || *coupon.StoreID != storeID || coupon.MerchantID != merchant.ID {
		return ErrCouponNotFound
	}
	if coupon.DeletedAt.Valid {
		return nil
	}

	return s.db.WithContext(ctx).Where("id = ?", couponID).Delete(&model.Coupon{}).Error
}

func (s *CouponService) ListForMerchant(ctx context.Context, userID, storeID int64) ([]model.Coupon, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreForbidden
		}
		return nil, err
	}

	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}
	if store.MerchantID != merchant.ID {
		return nil, ErrStoreForbidden
	}

	var coupons []model.Coupon
	if err := s.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Order("id desc").
		Find(&coupons).Error; err != nil {
		return nil, err
	}
	return coupons, nil
}

func (s *CouponService) loadOwnedCoupon(ctx context.Context, userID, storeID, couponID int64) (*model.Merchant, *model.Coupon, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrStoreForbidden
		}
		return nil, nil, err
	}

	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrStoreNotFound
		}
		return nil, nil, err
	}
	if store.MerchantID != merchant.ID {
		return nil, nil, ErrStoreForbidden
	}

	var coupon model.Coupon
	if err := s.db.WithContext(ctx).First(&coupon, couponID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrCouponNotFound
		}
		return nil, nil, err
	}
	if coupon.StoreID == nil || *coupon.StoreID != storeID || coupon.MerchantID != merchant.ID {
		return nil, nil, ErrCouponNotFound
	}
	return &merchant, &coupon, nil
}

func (s *CouponService) UpdateForStore(ctx context.Context, userID, storeID, couponID int64, input UpdateStoreCouponInput) (*model.Coupon, error) {
	merchant, coupon, err := s.loadOwnedCoupon(ctx, userID, storeID, couponID)
	if err != nil {
		return nil, err
	}

	// Validate that ValidFrom is not after ValidUntil (considering effective post-update values)
	effectiveFrom := coupon.ValidFrom
	if input.ValidFrom != nil {
		effectiveFrom = *input.ValidFrom
	}
	effectiveUntil := coupon.ValidUntil
	if input.ValidUntil != nil {
		effectiveUntil = *input.ValidUntil
	}
	if effectiveFrom != nil && effectiveUntil != nil && effectiveFrom.After(*effectiveUntil) {
		return nil, ErrInvalidCouponInput
	}

	effectivePrice := coupon.Price
	if input.Price != nil {
		effectivePrice = *input.Price
	}
	effectiveOriginalPrice := coupon.OriginalPrice
	if input.OriginalPrice != nil {
		effectiveOriginalPrice = *input.OriginalPrice
	}
	effectiveSalePrice := coupon.SalePrice
	if input.SalePrice != nil {
		effectiveSalePrice = *input.SalePrice
	}
	if input.SalePrice != nil && input.Price == nil {
		effectivePrice = effectiveSalePrice
	}
	effectiveDiscount := coupon.DiscountPercentage
	if input.DiscountPercentage != nil {
		effectiveDiscount = *input.DiscountPercentage
	}
	if err := validateCouponPricing(effectivePrice, effectiveOriginalPrice, effectiveSalePrice, effectiveDiscount); err != nil {
		return nil, err
	}
	if input.DishIDs != nil {
		if err := s.validateDishIDs(ctx, merchant.ID, *input.DishIDs); err != nil {
			return nil, err
		}
	}

	updates := map[string]interface{}{}
	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		if trimmed == "" {
			return nil, ErrInvalidCouponInput
		}
		updates["title"] = trimmed
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.CouponType != nil {
		updates["coupon_type"] = *input.CouponType
	}
	if input.ImageURL != nil {
		updates["image_url"] = *input.ImageURL
	}
	if input.Price != nil {
		updates["price"] = *input.Price
	}
	if input.OriginalPrice != nil {
		updates["original_price"] = *input.OriginalPrice
	}
	if input.SalePrice != nil {
		updates["sale_price"] = *input.SalePrice
		if input.Price == nil {
			// Keep the legacy price mirror in sync for partial sale-price edits.
			updates["price"] = *input.SalePrice
		}
	}
	if input.DiscountPercentage != nil {
		updates["discount_percentage"] = *input.DiscountPercentage
	}
	if input.DishIDs != nil {
		dishIDsJSON, err := json.Marshal(*input.DishIDs)
		if err != nil {
			return nil, err
		}
		updates["dish_ids"] = string(dishIDsJSON)
	}
	if input.TotalQuantity != nil {
		if *input.TotalQuantity <= 0 || *input.TotalQuantity < coupon.ClaimedCount {
			return nil, ErrInvalidCouponInput
		}
		updates["total_quantity"] = *input.TotalQuantity
	}
	if input.MaxPerUser != nil {
		if *input.MaxPerUser <= 0 {
			return nil, ErrInvalidCouponInput
		}
		updates["max_per_user"] = *input.MaxPerUser
	}
	if input.ValidFrom != nil {
		updates["valid_from"] = *input.ValidFrom
	}
	if input.ValidUntil != nil {
		updates["valid_until"] = *input.ValidUntil
		if *input.ValidUntil != nil {
			updates["expiry_date"] = **input.ValidUntil
		} else {
			updates["expiry_date"] = nil
		}
	}
	if input.Terms != nil {
		updates["terms"] = *input.Terms
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !isMerchantControlledCouponStatus(status) {
			return nil, ErrInvalidCouponInput
		}
		updates["status"] = status
	}
	if len(updates) == 0 {
		return coupon, nil
	}
	if err := s.db.WithContext(ctx).Model(&model.Coupon{}).Where("id = ?", couponID).Updates(updates).Error; err != nil {
		return nil, err
	}
	_, refreshed, err := s.loadOwnedCoupon(ctx, userID, storeID, couponID)
	return refreshed, err
}

func (s *CouponService) SetEnabled(ctx context.Context, userID, storeID, couponID int64, enabled bool) (*model.Coupon, error) {
	status := "disabled"
	if enabled {
		status = couponStatusActive
	}
	return s.UpdateForStore(ctx, userID, storeID, couponID, UpdateStoreCouponInput{Status: &status})
}

func (s *CouponService) ListPublishedByStore(ctx context.Context, storeID int64) ([]model.Coupon, error) {
	if _, err := s.ensurePublishedStore(ctx, storeID); err != nil {
		if errors.Is(err, ErrStoreNotPublished) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}

	now := time.Now()
	var coupons []model.Coupon
	if err := s.db.WithContext(ctx).
		Where("store_id = ? AND status = ?", storeID, couponStatusActive).
		Where("(valid_from IS NULL OR valid_from <= ?)", now).
		Where("(valid_until IS NULL OR valid_until >= ?)", now).
		Order("id DESC").
		Find(&coupons).Error; err != nil {
		return nil, err
	}

	// The persisted status is merchant-controlled. Filter derived states here
	// as well, otherwise a sold-out row can remain publicly advertised until a
	// background job rewrites its status.
	published := coupons[:0]
	for _, coupon := range coupons {
		if ComputeStatus(coupon, now) == couponStatusActive {
			published = append(published, coupon)
		}
	}
	return published, nil
}

func (s *CouponService) Validate(ctx context.Context, id int64, input ValidateInput) (*ValidateResult, error) {
	quantity := input.Quantity
	if quantity <= 0 {
		quantity = 1
	}

	coupon, err := s.loadCoupon(ctx, id)
	if err != nil {
		return nil, err
	}
	storeID, err := s.ensureCouponPurchasable(ctx, coupon)
	if err != nil {
		return nil, err
	}

	remaining := coupon.TotalQuantity - coupon.ClaimedCount
	if remaining < quantity {
		return nil, ErrCouponSoldOut
	}

	if input.UserID != nil && coupon.MaxPerUser > 0 {
		var claimedByUser int64
		if err := s.db.WithContext(ctx).
			Model(&model.Voucher{}).
			Where("user_id = ? AND coupon_id = ?", *input.UserID, coupon.ID).
			Count(&claimedByUser).Error; err != nil {
			return nil, err
		}
		if int(claimedByUser)+quantity > coupon.MaxPerUser {
			return nil, ErrCouponPerUserLimit
		}
	}

	return &ValidateResult{
		CouponID:    coupon.ID,
		StoreID:     storeID,
		MerchantID:  coupon.MerchantID,
		Quantity:    quantity,
		Remaining:   remaining,
		Price:       coupon.Price,
		MaxPerUser:  coupon.MaxPerUser,
		IsValid:     true,
		Status:      ComputeStatus(*coupon, time.Now()),
		Description: coupon.Description,
	}, nil
}

func isMerchantControlledCouponStatus(status string) bool {
	return status == couponStatusDraft || status == couponStatusActive || status == couponStatusDisabled
}

func validateCouponPricing(price, originalPrice, salePrice, discountPercentage float64) error {
	if price < 0 || originalPrice < 0 || salePrice < 0 || discountPercentage < 0 || discountPercentage > 100 {
		return ErrInvalidCouponInput
	}
	if originalPrice > 0 && salePrice > originalPrice {
		return ErrInvalidCouponInput
	}
	if price > 0 && salePrice > 0 && math.Abs(price-salePrice) > 0.0001 {
		return ErrInvalidCouponInput
	}
	return nil
}

func (s *CouponService) validateDishIDs(ctx context.Context, merchantID int64, dishIDs []int64) error {
	if len(dishIDs) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(dishIDs))
	for _, dishID := range dishIDs {
		if dishID <= 0 {
			return ErrInvalidCouponInput
		}
		if _, exists := seen[dishID]; exists {
			return ErrInvalidCouponInput
		}
		seen[dishID] = struct{}{}
	}

	var count int64
	if err := s.db.WithContext(ctx).
		Model(&model.Dish{}).
		Where("merchant_id = ? AND id IN ?", merchantID, dishIDs).
		Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(dishIDs)) {
		return ErrInvalidCouponInput
	}
	return nil
}

func (s *CouponService) InitiatePayment(ctx context.Context, couponID int64, userID string) error {
	_ = ctx
	_ = couponID
	_ = userID
	return ErrDeprecatedCouponPayment
}

func (s *CouponService) Redeem(ctx context.Context, couponID, userID int64) error {
	_ = ctx
	_ = couponID
	_ = userID
	return ErrDeprecatedCouponRedeem
}

func (s *CouponService) loadCoupon(ctx context.Context, id int64) (*model.Coupon, error) {
	var coupon model.Coupon
	if err := s.db.WithContext(ctx).First(&coupon, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponNotFound
		}
		return nil, err
	}
	return &coupon, nil
}

func (s *CouponService) ensurePublishedStore(ctx context.Context, storeID int64) (*model.Store, error) {
	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}
	if store.Status != storeStatusPublished {
		return nil, ErrStoreNotPublished
	}
	return &store, nil
}

func (s *CouponService) ensureCouponPurchasable(ctx context.Context, coupon *model.Coupon) (int64, error) {
	now := time.Now()
	if coupon.Status != couponStatusActive {
		return 0, ErrCouponInactive
	}
	if coupon.StoreID == nil {
		return 0, ErrCouponNotStoreScoped
	}
	if coupon.ValidFrom != nil && coupon.ValidFrom.After(now) {
		return 0, ErrCouponNotStarted
	}
	if coupon.ValidUntil != nil && coupon.ValidUntil.Before(now) {
		return 0, ErrCouponExpired
	}
	if !coupon.ExpiryDate.IsZero() && coupon.ExpiryDate.Before(now) {
		return 0, ErrCouponExpired
	}
	if coupon.TotalQuantity <= 0 {
		return 0, ErrCouponSoldOut
	}

	store, err := s.ensurePublishedStore(ctx, *coupon.StoreID)
	if err != nil {
		return 0, err
	}
	if store.MerchantID != coupon.MerchantID {
		return 0, ErrCouponStoreMismatch
	}
	return store.ID, nil
}
