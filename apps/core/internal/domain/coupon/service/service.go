package service

import (
	"context"
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
)

var (
	ErrCouponNotFound          = errors.New("coupon not found")
	ErrPackageNotFound         = errors.New("package not found")
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
	Title         string
	Description   string
	Type          string
	Price         float64
	TotalQuantity int
	MaxPerUser    int
	ValidFrom     *time.Time
	ValidUntil    *time.Time
	Terms         string
	Status        string
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

type ListPackagesQuery struct {
	Limit  int
	Cursor int64
}

type PackagePage struct {
	Data   []model.Package `json:"data"`
	Cursor *int64          `json:"cursor,omitempty"`
}

type PackageResponse struct {
	Data *model.Package `json:"data"`
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

// ListPackages returns active packages with their active, non-deleted coupons.
// Cursor pagination is ID-based and keeps the public response deterministic.
func (s *CouponService) ListPackages(ctx context.Context, query ListPackagesQuery) (*PackagePage, error) {
	limit, err := normalizePackagePageSize(query.Limit)
	if err != nil {
		return nil, err
	}
	if query.Cursor < 0 {
		return nil, ErrInvalidCouponInput
	}

	db := s.db.WithContext(ctx).
		Where("status = ?", "active").
		Preload("Coupons", func(couponDB *gorm.DB) *gorm.DB {
			return couponDB.Where("status = ?", couponStatusActive).Order("id desc")
		})
	if query.Cursor > 0 {
		db = db.Where("id < ?", query.Cursor)
	}

	rows := make([]model.Package, 0, limit)
	if err := db.Order("id desc").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}

	page := &PackagePage{Data: rows}
	if len(rows) > limit {
		rows = rows[:limit]
		page.Data = rows
		cursor := rows[len(rows)-1].ID
		page.Cursor = &cursor
	}
	return page, nil
}

// PackageDetail returns one active package and its active, non-deleted coupons.
func (s *CouponService) PackageDetail(ctx context.Context, id int64) (*model.Package, error) {
	if id <= 0 {
		return nil, ErrPackageNotFound
	}

	var pkg model.Package
	err := s.db.WithContext(ctx).
		Where("id = ? AND status = ?", id, "active").
		Preload("Coupons", func(couponDB *gorm.DB) *gorm.DB {
			return couponDB.Where("status = ?", couponStatusActive).Order("id desc")
		}).
		First(&pkg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPackageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &pkg, nil
}

func normalizePackagePageSize(limit int) (int, error) {
	if limit == 0 {
		return 20, nil
	}
	if limit < 1 || limit > 100 {
		return 0, ErrInvalidCouponInput
	}
	return limit, nil
}

func (s *CouponService) CreateForStore(ctx context.Context, userID, storeID int64, input CreateStoreCouponInput) (*model.Coupon, error) {
	title := strings.TrimSpace(input.Title)
	couponType := strings.TrimSpace(input.Type)
	if title == "" || couponType == "" || input.TotalQuantity <= 0 || input.Price < 0 {
		return nil, ErrInvalidCouponInput
	}
	if input.MaxPerUser <= 0 {
		return nil, ErrInvalidCouponInput
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && input.ValidFrom.After(*input.ValidUntil) {
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

	coupon := model.Coupon{
		MerchantID:    store.MerchantID,
		StoreID:       &store.ID,
		Title:         title,
		Description:   input.Description,
		Type:          couponType,
		Price:         input.Price,
		TotalQuantity: input.TotalQuantity,
		MaxPerUser:    input.MaxPerUser,
		Terms:         input.Terms,
		Status:        couponStatusActive,
	}
	if strings.TrimSpace(input.Status) != "" {
		coupon.Status = strings.TrimSpace(input.Status)
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
