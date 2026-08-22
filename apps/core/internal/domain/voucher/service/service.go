package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrVoucherNotFound         = errors.New("voucher not found")
	ErrVoucherForbidden        = errors.New("voucher forbidden")
	ErrVoucherNotRedeemable    = errors.New("voucher not redeemable")
	ErrVoucherExpired          = errors.New("voucher expired")
	ErrVoucherInvalidInput     = errors.New("invalid voucher input")
	ErrVoucherPaymentRequired  = errors.New("paid coupons require an order payment")
	ErrVoucherCouponInactive   = errors.New("coupon inactive")
	ErrVoucherCouponExpired    = errors.New("coupon expired")
	ErrVoucherCouponNotStarted = errors.New("coupon not started")
	ErrVoucherSoldOut          = errors.New("voucher coupon sold out")
	ErrVoucherPerUserLimit     = errors.New("voucher per-user limit exceeded")
	ErrVoucherInvalidStatus    = errors.New("invalid voucher status")
)

type CreateVoucherRequest struct {
	CouponID string `json:"couponId"`
}

type RedeemPreview struct {
	VoucherID     int64      `json:"voucher_id"`
	VoucherCode   string     `json:"voucher_code"`
	VoucherStatus string     `json:"voucher_status"`
	RedeemedAt    *time.Time `json:"redeemed_at,omitempty"`
	CouponID      int64      `json:"coupon_id"`
	CouponTitle   string     `json:"coupon_title"`
	StoreID       *int64     `json:"store_id,omitempty"`
	StoreName     string     `json:"store_name,omitempty"`
	MerchantID    int64      `json:"merchant_id"`
	MerchantName  string     `json:"merchant_name"`
	CanRedeem     bool       `json:"can_redeem"`
	Reason        string     `json:"reason,omitempty"`
}

type VoucherService struct {
	db *gorm.DB
}

func NewVoucherService(db *gorm.DB) *VoucherService {
	if db == nil {
		db = database.DB
	}
	return &VoucherService{db: db}
}

func (s *VoucherService) Create(ctx context.Context, userID int64, req CreateVoucherRequest) (model.Voucher, error) {
	couponID, err := strconv.ParseInt(strings.TrimSpace(req.CouponID), 10, 64)
	if userID <= 0 || err != nil || couponID <= 0 {
		return model.Voucher{}, ErrVoucherInvalidInput
	}

	var voucher model.Voucher
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var coupon model.Coupon
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&coupon, couponID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVoucherNotFound
			}
			return err
		}
		if coupon.Status != "active" {
			return ErrVoucherCouponInactive
		}
		now := time.Now()
		if coupon.ValidFrom != nil && coupon.ValidFrom.After(now) {
			return ErrVoucherCouponNotStarted
		}
		if coupon.ValidUntil != nil && coupon.ValidUntil.Before(now) {
			return ErrVoucherCouponExpired
		}
		if !coupon.ExpiryDate.IsZero() && coupon.ExpiryDate.Before(now) {
			return ErrVoucherCouponExpired
		}
		if coupon.Price > 0 || strings.EqualFold(coupon.Type, "paid") || strings.EqualFold(coupon.CouponType, "paid") {
			return ErrVoucherPaymentRequired
		}
		if coupon.TotalQuantity <= coupon.ClaimedCount {
			return ErrVoucherSoldOut
		}

		if coupon.MaxPerUser > 0 {
			var claimedByUser int64
			if err := tx.Model(&model.Voucher{}).
				Where("user_id = ? AND coupon_id = ?", userID, coupon.ID).
				Count(&claimedByUser).Error; err != nil {
				return err
			}
			if int(claimedByUser) >= coupon.MaxPerUser {
				return ErrVoucherPerUserLimit
			}
		}

		updated := tx.Model(&model.Coupon{}).
			Where("id = ? AND (total_quantity - claimed_count) > 0", coupon.ID).
			UpdateColumn("claimed_count", gorm.Expr("claimed_count + 1"))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return ErrVoucherSoldOut
		}

		scanToken, err := generateVoucherScanToken()
		if err != nil {
			return err
		}
		merchantID := coupon.MerchantID
		voucher = model.Voucher{
			Code:       generateVoucherCode(),
			ScanToken:  scanToken,
			CouponID:   coupon.ID,
			UserID:     userID,
			MerchantID: &merchantID,
			Status:     "active",
			ExpiryDate: coupon.ExpiryDate,
			ValidFrom:  coupon.ValidFrom,
			ValidUntil: coupon.ValidUntil,
		}
		if voucher.ExpiryDate.IsZero() && voucher.ValidUntil != nil {
			voucher.ExpiryDate = *voucher.ValidUntil
		}
		return tx.Create(&voucher).Error
	})
	return voucher, err
}

func (s *VoucherService) List(ctx context.Context, userID int64) ([]model.Voucher, error) {
	var list []model.Voucher
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *VoucherService) Detail(ctx context.Context, id int64) (*model.Voucher, error) {
	var v model.Voucher
	if err := s.db.WithContext(ctx).First(&v, id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *VoucherService) DetailForUser(ctx context.Context, userID, id int64) (*model.Voucher, error) {
	var v model.Voucher
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *VoucherService) ByCode(ctx context.Context, code string) (*model.Voucher, error) {
	var v model.Voucher
	if err := s.db.WithContext(ctx).Where("code = ?", code).First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *VoucherService) ByCodeForUser(ctx context.Context, userID int64, code string) (*model.Voucher, error) {
	var v model.Voucher
	if err := s.db.WithContext(ctx).Where("code = ? AND user_id = ?", code, userID).First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *VoucherService) Use(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return ErrVoucherInvalidInput
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var voucher model.Voucher
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&voucher, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVoucherNotFound
			}
			return err
		}
		if voucher.UserID != userID {
			return ErrVoucherForbidden
		}
		if voucher.Status != "active" {
			return ErrVoucherNotRedeemable
		}
		now := time.Now()
		if voucher.ValidUntil != nil && voucher.ValidUntil.Before(now) {
			return ErrVoucherExpired
		}
		if !voucher.ExpiryDate.IsZero() && voucher.ExpiryDate.Before(now) {
			return ErrVoucherExpired
		}

		updated := tx.Model(&model.Voucher{}).
			Where("id = ? AND user_id = ? AND status = ?", id, userID, "active").
			Updates(map[string]interface{}{
				"status":      "used",
				"redeemed_at": now,
				"redeemed_by": userID,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return ErrVoucherNotRedeemable
		}
		return tx.Unscoped().Model(&model.Coupon{}).
			Where("id = ?", voucher.CouponID).
			UpdateColumn("redeemed_count", gorm.Expr("redeemed_count + 1")).Error
	})
}

func (s *VoucherService) UpdateStatus(ctx context.Context, userID, id int64, status string) error {
	if status != "used" {
		return ErrVoucherInvalidStatus
	}
	return s.Use(ctx, userID, id)
}

func (s *VoucherService) PreviewRedeemByToken(ctx context.Context, merchantUserID int64, scanToken string) (*RedeemPreview, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", merchantUserID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherForbidden
		}
		return nil, err
	}

	var voucher model.Voucher
	if err := s.db.WithContext(ctx).Where("scan_token = ?", scanToken).First(&voucher).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherNotFound
		}
		return nil, err
	}

	var coupon model.Coupon
	if err := s.db.WithContext(ctx).Unscoped().First(&coupon, voucher.CouponID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherNotFound
		}
		return nil, err
	}
	if coupon.StoreID == nil || coupon.MerchantID != merchant.ID {
		return nil, ErrVoucherForbidden
	}

	preview := &RedeemPreview{
		VoucherID:     voucher.ID,
		VoucherCode:   voucher.Code,
		VoucherStatus: voucher.Status,
		RedeemedAt:    voucher.RedeemedAt,
		CouponID:      coupon.ID,
		CouponTitle:   coupon.Title,
		StoreID:       coupon.StoreID,
		MerchantID:    merchant.ID,
		MerchantName:  merchant.Name,
	}

	if coupon.StoreID != nil {
		var store model.Store
		if err := s.db.WithContext(ctx).Unscoped().First(&store, *coupon.StoreID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		} else {
			preview.StoreName = store.Name
		}
	}

	now := time.Now()
	switch {
	case voucher.Status != "active":
		preview.CanRedeem = false
		if voucher.Status == "used" {
			preview.Reason = "used"
		} else {
			preview.Reason = "not_redeemable"
		}
	case voucher.ValidUntil != nil && voucher.ValidUntil.Before(now):
		preview.CanRedeem = false
		preview.Reason = "expired"
	case !voucher.ExpiryDate.IsZero() && voucher.ExpiryDate.Before(now):
		preview.CanRedeem = false
		preview.Reason = "expired"
	default:
		preview.CanRedeem = true
	}

	return preview, nil
}

func (s *VoucherService) RedeemByMerchantToken(ctx context.Context, merchantUserID int64, scanToken string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var merchant model.Merchant
		if err := tx.Where("user_id = ?", merchantUserID).First(&merchant).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVoucherForbidden
			}
			return err
		}

		var voucher model.Voucher
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("scan_token = ?", scanToken).
			First(&voucher).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVoucherNotFound
			}
			return err
		}

		var coupon model.Coupon
		if err := tx.Unscoped().First(&coupon, voucher.CouponID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVoucherNotFound
			}
			return err
		}
		if coupon.StoreID == nil || coupon.MerchantID != merchant.ID {
			return ErrVoucherForbidden
		}

		now := time.Now()
		if voucher.Status != "active" {
			return ErrVoucherNotRedeemable
		}
		if voucher.ValidUntil != nil && voucher.ValidUntil.Before(now) {
			return ErrVoucherExpired
		}
		if !voucher.ExpiryDate.IsZero() && voucher.ExpiryDate.Before(now) {
			return ErrVoucherExpired
		}

		if err := tx.Model(&model.Voucher{}).
			Where("id = ?", voucher.ID).
			Updates(map[string]interface{}{
				"status":      "used",
				"redeemed_at": now,
				"redeemed_by": merchantUserID,
			}).Error; err != nil {
			return err
		}

		return tx.Unscoped().Model(&model.Coupon{}).
			Where("id = ?", coupon.ID).
			UpdateColumn("redeemed_count", gorm.Expr("redeemed_count + 1")).Error
	})
}

func (s *VoucherService) RedeemByMerchant(ctx context.Context, userID, voucherID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var merchant model.Merchant
		if err := tx.Where("user_id = ?", userID).First(&merchant).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVoucherForbidden
			}
			return err
		}

		var voucher model.Voucher
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&voucher, voucherID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVoucherNotFound
			}
			return err
		}

		var coupon model.Coupon
		if err := tx.Unscoped().First(&coupon, voucher.CouponID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVoucherNotFound
			}
			return err
		}
		if coupon.StoreID == nil || coupon.MerchantID != merchant.ID {
			return ErrVoucherForbidden
		}

		now := time.Now()
		if voucher.Status != "active" {
			return ErrVoucherNotRedeemable
		}
		if voucher.ValidUntil != nil && voucher.ValidUntil.Before(now) {
			return ErrVoucherExpired
		}
		if !voucher.ExpiryDate.IsZero() && voucher.ExpiryDate.Before(now) {
			return ErrVoucherExpired
		}

		if err := tx.Model(&model.Voucher{}).
			Where("id = ?", voucher.ID).
			Updates(map[string]interface{}{
				"status":      "used",
				"redeemed_at": now,
				"redeemed_by": userID,
			}).Error; err != nil {
			return err
		}

		return tx.Unscoped().Model(&model.Coupon{}).
			Where("id = ?", coupon.ID).
			UpdateColumn("redeemed_count", gorm.Expr("redeemed_count + 1")).Error
	})
}

func generateVoucherScanToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate voucher scan token: %w", err)
	}
	return "vst_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func generateVoucherCode() string {
	raw := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return "VCH-" + raw[:12]
}
