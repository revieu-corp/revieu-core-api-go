package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/observability"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrVoucherNotFound      = errors.New("voucher not found")
	ErrVoucherForbidden     = errors.New("voucher forbidden")
	ErrVoucherNotRedeemable = errors.New("voucher not redeemable")
	ErrVoucherExpired       = errors.New("voucher expired")
)

type CreateVoucherRequest struct {
	CouponID string `json:"couponId"`
	UserID   string `json:"userId"`
	Code     string `json:"code"`
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

func (s *VoucherService) Create(ctx context.Context, req CreateVoucherRequest) (model.Voucher, error) {
	couponID, _ := strconv.ParseInt(req.CouponID, 10, 64)
	userID, _ := strconv.ParseInt(req.UserID, 10, 64)

	// Look up coupon to get merchant ID
	var coupon model.Coupon
	var merchantID *int64
	if err := s.db.WithContext(ctx).First(&coupon, couponID).Error; err == nil {
		merchantID = &coupon.MerchantID
	}

	scanToken, err := generateVoucherScanToken()
	if err != nil {
		return model.Voucher{}, err
	}

	v := model.Voucher{
		Code:       req.Code,
		ScanToken:  scanToken,
		CouponID:   couponID,
		UserID:     userID,
		MerchantID: merchantID,
		Status:     "active",
	}
	return v, s.db.WithContext(ctx).Create(&v).Error
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

func (s *VoucherService) Use(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Model(&model.Voucher{}).Where("id = ?", id).UpdateColumn("status", "used").Error
}

func (s *VoucherService) UpdateStatus(ctx context.Context, id int64, status string) error {
	return s.db.WithContext(ctx).Model(&model.Voucher{}).Where("id = ?", id).UpdateColumn("status", status).Error
}

func (s *VoucherService) PreviewRedeemByToken(ctx context.Context, merchantUserID int64, scanToken string) (*RedeemPreview, error) {
	var voucher model.Voucher
	if err := s.db.WithContext(ctx).Where("scan_token = ?", scanToken).First(&voucher).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherNotFound
		}
		return nil, err
	}

	return s.previewRedeemVoucher(ctx, merchantUserID, voucher)
}

func (s *VoucherService) PreviewRedeemByCode(ctx context.Context, merchantUserID int64, code string) (*RedeemPreview, error) {
	var voucher model.Voucher
	if err := s.db.WithContext(ctx).Where("code = ?", code).First(&voucher).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherNotFound
		}
		return nil, err
	}

	return s.previewRedeemVoucher(ctx, merchantUserID, voucher)
}

func (s *VoucherService) previewRedeemVoucher(ctx context.Context, merchantUserID int64, voucher model.Voucher) (*RedeemPreview, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", merchantUserID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVoucherForbidden
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

func (s *VoucherService) RedeemByMerchantCode(ctx context.Context, merchantUserID int64, code string) error {
	var voucher model.Voucher
	if err := s.db.WithContext(ctx).Where("code = ?", code).First(&voucher).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVoucherNotFound
		}
		return err
	}

	return s.redeemByMerchant(ctx, merchantUserID, voucher.ID, "voucher.redeem_by_code")
}

func (s *VoucherService) RedeemByMerchantToken(ctx context.Context, merchantUserID int64, scanToken string) error {
	started := time.Now()
	var voucherID int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		voucherID = voucher.ID

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
	duration := time.Since(started)
	s.recordRedemptionOutcome(ctx, merchantUserID, voucherID, "voucher.redeem_by_token", err, duration)
	return err
}

func (s *VoucherService) RedeemByMerchant(ctx context.Context, userID, voucherID int64) error {
	return s.redeemByMerchant(ctx, userID, voucherID, "voucher.redeem")
}

func (s *VoucherService) redeemByMerchant(ctx context.Context, userID, voucherID int64, action string) error {
	started := time.Now()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	duration := time.Since(started)
	s.recordRedemptionOutcome(ctx, userID, voucherID, action, err, duration)
	return err
}

func (s *VoucherService) recordRedemptionOutcome(ctx context.Context, userID, voucherID int64, action string, err error, duration time.Duration) {
	audit := observability.AuditInput{
		ActorID:    userID,
		ActorRole:  "merchant",
		Action:     action,
		TargetType: "voucher",
		TargetID:   voucherID,
		Result:     observability.ResultSuccess,
		Details:    `{"status":"used"}`,
		Duration:   duration,
	}
	if err != nil {
		audit.Result = observability.ResultFailure
		audit.ErrorClass = observability.ClassifyError(err)
	}
	_ = observability.WriteAudit(ctx, s.db, audit)
	observability.RecordTransaction(ctx, action, err == nil, err, duration)
}

func generateVoucherScanToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate voucher scan token: %w", err)
	}
	return "vst_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
