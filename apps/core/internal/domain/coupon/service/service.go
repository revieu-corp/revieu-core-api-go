package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

const (
	storeStatusPublished int16 = 1
	couponStatusActive         = "active"
	couponStatusDraft          = "draft"
	couponStatusDisabled       = "disabled"
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
	ErrMerchantForbidden       = errors.New("merchant forbidden")
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
	OriginalPrice      *float64
	SalePrice          *float64
	DiscountPercentage *float64
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
	Type               *string
	CouponType         *string
	Price              *float64
	OriginalPrice      *float64
	SalePrice          *float64
	DiscountPercentage *float64
	ImageURL           *string
	DishIDs            *[]int64
	TotalQuantity      *int
	MaxPerUser         *int
	ValidFrom          *time.Time
	ValidUntil         *time.Time
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

type ListMerchantCouponsQuery struct {
	Status          string
	StoreID         *int64
	ValidFromBefore *time.Time
	ValidUntilAfter *time.Time
	Limit           int
	Cursor          int64
}

type CouponPage struct {
	Data   []model.Coupon `json:"data"`
	Cursor *int64         `json:"cursor,omitempty"`
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

// ListForMerchant returns the authenticated merchant's live coupons, including
// drafts and disabled coupons needed by the merchant console.
func (s *CouponService) ListForMerchant(ctx context.Context, userID int64, query ListMerchantCouponsQuery) (*CouponPage, error) {
	merchant, err := s.ensureMerchantPrincipal(ctx, userID, false)
	if err != nil {
		return nil, err
	}
	limit, err := normalizeCouponPageSize(query.Limit)
	if err != nil || query.Cursor < 0 {
		return nil, ErrInvalidCouponInput
	}

	status := strings.ToLower(strings.TrimSpace(query.Status))
	if status != "" && (status != couponStatusActive && status != couponStatusDraft && status != couponStatusDisabled) {
		return nil, ErrInvalidCouponInput
	}
	if query.StoreID != nil && *query.StoreID <= 0 {
		return nil, ErrInvalidCouponInput
	}
	if query.ValidFromBefore != nil && query.ValidUntilAfter != nil && query.ValidFromBefore.After(*query.ValidUntilAfter) {
		return nil, ErrInvalidCouponInput
	}

	db := s.db.WithContext(ctx).Where("merchant_id = ?", merchant.ID)
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if query.StoreID != nil {
		db = db.Where("store_id = ?", *query.StoreID)
	}
	if query.ValidFromBefore != nil {
		db = db.Where("(valid_from IS NULL OR valid_from <= ?)", *query.ValidFromBefore)
	}
	if query.ValidUntilAfter != nil {
		db = db.Where("(valid_until IS NULL OR valid_until >= ?)", *query.ValidUntilAfter)
	}
	if query.Cursor > 0 {
		db = db.Where("id < ?", query.Cursor)
	}

	rows := make([]model.Coupon, 0, limit)
	if err := db.Order("id desc").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	page := &CouponPage{Data: rows}
	if len(rows) > limit {
		rows = rows[:limit]
		page.Data = rows
		cursor := rows[len(rows)-1].ID
		page.Cursor = &cursor
	}
	return page, nil
}

// UpdateForMerchant updates editable fields for an owned store-scoped coupon.
// Lifecycle changes use SetStatusForMerchant so publish/deactivate transitions
// cannot be hidden inside an arbitrary PATCH.
func (s *CouponService) UpdateForMerchant(ctx context.Context, userID, couponID int64, input UpdateStoreCouponInput) (*model.Coupon, error) {
	if input.Status != nil {
		return nil, ErrInvalidCouponInput
	}
	merchant, err := s.ensureMerchantPrincipal(ctx, userID, false)
	if err != nil {
		return nil, err
	}
	var coupon model.Coupon
	if err := s.db.WithContext(ctx).Where("id = ? AND merchant_id = ?", couponID, merchant.ID).First(&coupon).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponNotFound
		}
		return nil, err
	}
	if coupon.StoreID == nil {
		return nil, ErrCouponNotStoreScoped
	}
	return s.UpdateForStore(ctx, userID, *coupon.StoreID, coupon.ID, input)
}

// SetStatusForMerchant applies the explicit activate/deactivate lifecycle
// actions and makes activation a verified-merchant operation.
func (s *CouponService) SetStatusForMerchant(ctx context.Context, userID, couponID int64, status string) (*model.Coupon, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != couponStatusActive && status != couponStatusDisabled {
		return nil, ErrInvalidCouponInput
	}
	merchant, err := s.ensureMerchantPrincipal(ctx, userID, status == couponStatusActive)
	if err != nil {
		return nil, err
	}

	var coupon model.Coupon
	if err := s.db.WithContext(ctx).Where("id = ? AND merchant_id = ?", couponID, merchant.ID).First(&coupon).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponNotFound
		}
		return nil, err
	}
	if status == couponStatusActive {
		now := time.Now()
		if coupon.ValidUntil != nil && coupon.ValidUntil.Before(now) {
			return nil, ErrCouponExpired
		}
		if coupon.TotalQuantity-coupon.ClaimedCount <= 0 {
			return nil, ErrCouponSoldOut
		}
		if coupon.StoreID != nil {
			var store model.Store
			if err := s.db.WithContext(ctx).First(&store, *coupon.StoreID).Error; err != nil {
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
		}
	}
	if coupon.Status == status {
		return &coupon, nil
	}
	if err := s.db.WithContext(ctx).Model(&model.Coupon{}).
		Where("id = ? AND merchant_id = ?", coupon.ID, merchant.ID).
		Update("status", status).Error; err != nil {
		return nil, err
	}
	coupon.Status = status
	return &coupon, nil
}

func (s *CouponService) CreateForStore(ctx context.Context, userID, storeID int64, input CreateStoreCouponInput) (*model.Coupon, error) {
	title := strings.TrimSpace(input.Title)
	couponType := strings.TrimSpace(input.Type)
	merchantCouponType := strings.TrimSpace(input.CouponType)
	if merchantCouponType == "" {
		merchantCouponType = "normal"
	}
	if title == "" || couponType == "" || input.TotalQuantity <= 0 || input.Price < 0 {
		return nil, ErrInvalidCouponInput
	}
	if input.MaxPerUser <= 0 {
		return nil, ErrInvalidCouponInput
	}
	if merchantCouponType != "normal" && merchantCouponType != "limited_time" {
		return nil, ErrInvalidCouponInput
	}
	if merchantCouponType == "limited_time" && (input.ValidFrom == nil || input.ValidUntil == nil) {
		return nil, ErrInvalidCouponInput
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && input.ValidFrom.After(*input.ValidUntil) {
		return nil, ErrInvalidCouponInput
	}
	originalPrice, salePrice, discountPercentage, err := resolveCouponPrices(input.Price, input.OriginalPrice, input.SalePrice, input.DiscountPercentage)
	if err != nil {
		return nil, err
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
	dishIDs, err := encodeOwnedDishIDs(ctx, s.db, merchant.ID, input.DishIDs)
	if err != nil {
		return nil, err
	}
	status := normalizeCouponStatus(input.Status)
	if status == "" {
		return nil, ErrInvalidCouponInput
	}

	coupon := model.Coupon{
		MerchantID:         store.MerchantID,
		StoreID:            &store.ID,
		Title:              title,
		Description:        input.Description,
		Type:               couponType,
		Price:              salePrice,
		CouponType:         merchantCouponType,
		ImageURL:           strings.TrimSpace(input.ImageURL),
		OriginalPrice:      originalPrice,
		SalePrice:          salePrice,
		DiscountPercentage: discountPercentage,
		DishIDs:            dishIDs,
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

// ListForStore returns every live coupon owned by the merchant, including
// drafts and disabled coupons that the merchant must be able to edit.
func (s *CouponService) ListForStore(ctx context.Context, userID, storeID int64) ([]model.Coupon, error) {
	merchantID, err := s.ensureOwnedStore(ctx, userID, storeID)
	if err != nil {
		return nil, err
	}
	var coupons []model.Coupon
	if err := s.db.WithContext(ctx).
		Where("merchant_id = ? AND store_id = ?", merchantID, storeID).
		Order("id DESC").
		Find(&coupons).Error; err != nil {
		return nil, err
	}
	return coupons, nil
}

func (s *CouponService) UpdateForStore(ctx context.Context, userID, storeID, couponID int64, input UpdateStoreCouponInput) (*model.Coupon, error) {
	merchantID, err := s.ensureOwnedStore(ctx, userID, storeID)
	if err != nil {
		return nil, err
	}
	var coupon model.Coupon
	if err := s.db.WithContext(ctx).Where("id = ? AND merchant_id = ? AND store_id = ?", couponID, merchantID, storeID).First(&coupon).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponStoreMismatch
		}
		return nil, err
	}

	updates := map[string]interface{}{}
	if input.Title != nil {
		if strings.TrimSpace(*input.Title) == "" {
			return nil, ErrInvalidCouponInput
		}
		updates["title"] = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.Type != nil {
		if strings.TrimSpace(*input.Type) == "" {
			return nil, ErrInvalidCouponInput
		}
		updates["type"] = strings.TrimSpace(*input.Type)
	}
	if input.CouponType != nil {
		couponType := strings.TrimSpace(*input.CouponType)
		if couponType != "normal" && couponType != "limited_time" {
			return nil, ErrInvalidCouponInput
		}
		updates["coupon_type"] = couponType
	}
	if input.Price != nil {
		if *input.Price < 0 {
			return nil, ErrInvalidCouponInput
		}
		updates["price"] = *input.Price
	}
	if input.OriginalPrice != nil {
		if *input.OriginalPrice < 0 {
			return nil, ErrInvalidCouponInput
		}
		updates["original_price"] = *input.OriginalPrice
	}
	if input.SalePrice != nil {
		if *input.SalePrice < 0 {
			return nil, ErrInvalidCouponInput
		}
		updates["sale_price"] = *input.SalePrice
		updates["price"] = *input.SalePrice
	}
	if input.DiscountPercentage != nil {
		if *input.DiscountPercentage < 0 || *input.DiscountPercentage > 100 {
			return nil, ErrInvalidCouponInput
		}
		updates["discount_percentage"] = *input.DiscountPercentage
	}
	if input.ImageURL != nil {
		updates["image_url"] = strings.TrimSpace(*input.ImageURL)
	}
	if input.DishIDs != nil {
		dishIDs, encodeErr := encodeOwnedDishIDs(ctx, s.db, merchantID, *input.DishIDs)
		if encodeErr != nil {
			return nil, encodeErr
		}
		updates["dish_ids"] = dishIDs
	}
	if input.TotalQuantity != nil {
		if *input.TotalQuantity <= 0 {
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
		updates["valid_from"] = input.ValidFrom
		updates["expiry_date"] = input.ValidUntil
	}
	if input.ValidUntil != nil {
		updates["valid_until"] = input.ValidUntil
		updates["expiry_date"] = input.ValidUntil
	}
	if input.Terms != nil {
		updates["terms"] = *input.Terms
	}
	if input.Status != nil {
		status := normalizeCouponStatus(*input.Status)
		if status == "" {
			return nil, ErrInvalidCouponInput
		}
		updates["status"] = status
	}

	finalCouponType := coupon.CouponType
	if input.CouponType != nil {
		finalCouponType = strings.TrimSpace(*input.CouponType)
	}
	finalFrom, finalUntil := coupon.ValidFrom, coupon.ValidUntil
	if input.ValidFrom != nil {
		finalFrom = input.ValidFrom
	}
	if input.ValidUntil != nil {
		finalUntil = input.ValidUntil
	}
	if finalCouponType == "limited_time" && (finalFrom == nil || finalUntil == nil) {
		return nil, ErrInvalidCouponInput
	}
	if finalFrom != nil && finalUntil != nil && finalFrom.After(*finalUntil) {
		return nil, ErrInvalidCouponInput
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&model.Coupon{}).Where("id = ?", coupon.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	var updated model.Coupon
	if err := s.db.WithContext(ctx).First(&updated, coupon.ID).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *CouponService) SetStatusForStore(ctx context.Context, userID, storeID, couponID int64, status string) (*model.Coupon, error) {
	status = normalizeCouponStatus(status)
	if status != couponStatusActive && status != couponStatusDisabled {
		return nil, ErrInvalidCouponInput
	}
	merchantID, err := s.ensureOwnedStore(ctx, userID, storeID)
	if err != nil {
		return nil, err
	}
	var coupon model.Coupon
	if err := s.db.WithContext(ctx).Where("id = ? AND merchant_id = ? AND store_id = ?", couponID, merchantID, storeID).First(&coupon).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponStoreMismatch
		}
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.Coupon{}).Where("id = ?", coupon.ID).Update("status", status).Error; err != nil {
		return nil, err
	}
	coupon.Status = status
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

func (s *CouponService) ensureOwnedStore(ctx context.Context, userID, storeID int64) (int64, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrStoreForbidden
		}
		return 0, err
	}
	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrStoreNotFound
		}
		return 0, err
	}
	if store.MerchantID != merchant.ID {
		return 0, ErrStoreForbidden
	}
	return merchant.ID, nil
}

func (s *CouponService) ensureMerchantPrincipal(ctx context.Context, userID int64, requireVerified bool) (*model.Merchant, error) {
	if userID <= 0 {
		return nil, ErrMerchantForbidden
	}
	var user model.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMerchantForbidden
		}
		return nil, err
	}
	if user.Status != 0 || strings.ToLower(strings.TrimSpace(user.Role)) != "merchant" {
		return nil, ErrMerchantForbidden
	}

	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMerchantForbidden
		}
		return nil, err
	}
	if requireVerified && strings.ToLower(strings.TrimSpace(merchant.VerificationStatus)) != "verified" {
		return nil, ErrMerchantForbidden
	}
	return &merchant, nil
}

func normalizeCouponPageSize(limit int) (int, error) {
	if limit == 0 {
		return 20, nil
	}
	if limit < 1 || limit > 100 {
		return 0, ErrInvalidCouponInput
	}
	return limit, nil
}

func resolveCouponPrices(price float64, original, sale, discount *float64) (float64, float64, float64, error) {
	if price < 0 {
		return 0, 0, 0, ErrInvalidCouponInput
	}
	salePrice := price
	if sale != nil {
		salePrice = *sale
	}
	originalPrice := float64(0)
	if original != nil {
		originalPrice = *original
	}
	if originalPrice < 0 || salePrice < 0 || (original != nil && salePrice > originalPrice && originalPrice > 0) {
		return 0, 0, 0, ErrInvalidCouponInput
	}
	discountPercentage := float64(0)
	if discount != nil {
		discountPercentage = *discount
	} else if originalPrice > 0 {
		discountPercentage = ((originalPrice - salePrice) / originalPrice) * 100
	}
	if discountPercentage < 0 || discountPercentage > 100 {
		return 0, 0, 0, ErrInvalidCouponInput
	}
	return originalPrice, salePrice, discountPercentage, nil
}

func normalizeCouponStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "", couponStatusActive:
		return couponStatusActive
	case couponStatusDraft, couponStatusDisabled:
		return strings.TrimSpace(status)
	default:
		return ""
	}
}

func encodeOwnedDishIDs(ctx context.Context, db *gorm.DB, merchantID int64, ids []int64) (string, error) {
	if len(ids) == 0 {
		encoded, err := json.Marshal([]int64{})
		return string(encoded), err
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.Dish{}).Where("merchant_id = ? AND id IN ?", merchantID, ids).Count(&count).Error; err != nil {
		return "", err
	}
	if count != int64(len(ids)) {
		return "", ErrInvalidCouponInput
	}
	encoded, err := json.Marshal(ids)
	return string(encoded), err
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
	return coupons, nil
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
		Status:      coupon.Status,
		Description: coupon.Description,
	}, nil
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
