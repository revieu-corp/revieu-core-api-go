package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	notificationservice "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/notification/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	storeStatusPublished int16 = 1
	couponStatusActive         = "active"
	orderStatusPending         = "pending"
	orderStatusPaid            = "paid"
	voucherStatusActive        = "active"
	paymentStatusSuccess       = "success"
)

var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderForbidden      = errors.New("order forbidden")
	ErrOrderInvalidInput   = errors.New("invalid order input")
	ErrOrderInvalidState   = errors.New("invalid order state")
	ErrStoreNotFound       = errors.New("store not found")
	ErrStoreNotPublished   = errors.New("store not published")
	ErrCouponNotFound      = errors.New("coupon not found")
	ErrCouponInactive      = errors.New("coupon inactive")
	ErrCouponNotStarted    = errors.New("coupon not started")
	ErrCouponExpired       = errors.New("coupon expired")
	ErrCouponSoldOut       = errors.New("coupon sold out")
	ErrCouponNotStoreScope = errors.New("coupon must be store scoped")
	ErrCouponStoreMismatch = errors.New("coupon store mismatch")
	ErrCouponPerUserLimit  = errors.New("coupon per-user limit exceeded")
)

type CreateOrderInput struct {
	CouponID int64 `json:"coupon_id"`
	Quantity int   `json:"quantity"`
}

type OrderDetail struct {
	Order    model.Order     `json:"order"`
	Vouchers []model.Voucher `json:"vouchers"`
}

type PayResult struct {
	Order    model.Order     `json:"order"`
	Vouchers []model.Voucher `json:"vouchers"`
}

type OrderService struct {
	db *gorm.DB
}

func NewOrderService(db *gorm.DB) *OrderService {
	if db == nil {
		db = database.DB
	}
	return &OrderService{db: db}
}

func (s *OrderService) Create(ctx context.Context, userID int64, input CreateOrderInput) (*model.Order, error) {
	if input.CouponID <= 0 {
		return nil, ErrOrderInvalidInput
	}

	quantity := input.Quantity
	if quantity <= 0 {
		quantity = 1
	}

	coupon, store, err := s.loadPurchasableCoupon(ctx, input.CouponID)
	if err != nil {
		return nil, err
	}

	if coupon.TotalQuantity-coupon.ClaimedCount < quantity {
		return nil, ErrCouponSoldOut
	}

	if coupon.MaxPerUser > 0 {
		var claimedByUser int64
		if err := s.db.WithContext(ctx).
			Model(&model.Voucher{}).
			Where("user_id = ? AND coupon_id = ?", userID, coupon.ID).
			Count(&claimedByUser).Error; err != nil {
			return nil, err
		}
		if int(claimedByUser)+quantity > coupon.MaxPerUser {
			return nil, ErrCouponPerUserLimit
		}
	}

	order := model.Order{
		UserID:     userID,
		CouponID:   &coupon.ID,
		MerchantID: &coupon.MerchantID,
		StoreID:    &store.ID,
		Quantity:   quantity,
		TotalPrice: coupon.Price * float64(quantity),
		Status:     orderStatusPending,
	}
	if err := s.db.WithContext(ctx).Create(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (s *OrderService) List(ctx context.Context, userID int64) ([]model.Order, error) {
	var orders []model.Order
	if err := s.db.WithContext(ctx).
		Preload("Coupon").
		Where("user_id = ?", userID).
		Order("id DESC").
		Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func (s *OrderService) Detail(ctx context.Context, userID, orderID int64) (*OrderDetail, error) {
	var order model.Order
	if err := s.db.WithContext(ctx).Preload("Coupon").First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	if order.UserID != userID {
		return nil, ErrOrderForbidden
	}

	var vouchers []model.Voucher
	if err := s.db.WithContext(ctx).
		Where("order_id = ?", order.ID).
		Order("id ASC").
		Find(&vouchers).Error; err != nil {
		return nil, err
	}

	return &OrderDetail{Order: order, Vouchers: vouchers}, nil
}

func (s *OrderService) Pay(ctx context.Context, userID, orderID int64) (*PayResult, error) {
	var result *PayResult
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}
		if order.UserID != userID {
			return ErrOrderForbidden
		}
		if order.CouponID == nil {
			return ErrOrderInvalidState
		}

		if order.Status == orderStatusPaid {
			var existing []model.Voucher
			if err := tx.Where("order_id = ?", order.ID).Order("id ASC").Find(&existing).Error; err != nil {
				return err
			}
			if err := createOrderPaidNotification(ctx, tx, order); err != nil {
				return err
			}
			result = &PayResult{Order: order, Vouchers: existing}
			return nil
		}
		if order.Status != orderStatusPending {
			return ErrOrderInvalidState
		}
		if order.Quantity <= 0 {
			return ErrOrderInvalidState
		}

		coupon, _, err := s.loadPurchasableCouponTx(tx, *order.CouponID)
		if err != nil {
			return err
		}
		if coupon.TotalQuantity-coupon.ClaimedCount < order.Quantity {
			return ErrCouponSoldOut
		}

		if coupon.MaxPerUser > 0 {
			var claimedByUser int64
			if err := tx.Model(&model.Voucher{}).
				Where("user_id = ? AND coupon_id = ?", userID, coupon.ID).
				Count(&claimedByUser).Error; err != nil {
				return err
			}
			if int(claimedByUser)+order.Quantity > coupon.MaxPerUser {
				return ErrCouponPerUserLimit
			}
		}

		update := tx.Model(&model.Coupon{}).
			Where("id = ? AND (total_quantity - claimed_count) >= ?", coupon.ID, order.Quantity).
			UpdateColumn("claimed_count", gorm.Expr("claimed_count + ?", order.Quantity))
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return ErrCouponSoldOut
		}

		now := time.Now()
		merchantID := coupon.MerchantID
		userIDCopy := userID
		payment := model.Payment{
			Amount:           order.TotalPrice,
			Currency:         "USD",
			Status:           paymentStatusSuccess,
			CouponID:         order.CouponID,
			MerchantID:       &merchantID,
			OrderID:          &order.ID,
			UserID:           &userIDCopy,
			PaymentMethod:    "mock",
			PaymentSessionID: fmt.Sprintf("mock-order-%d", order.ID),
			PaidAt:           &now,
		}
		if err := tx.Create(&payment).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.Order{}).
			Where("id = ?", order.ID).
			UpdateColumn("status", orderStatusPaid).Error; err != nil {
			return err
		}
		order.Status = orderStatusPaid

		vouchers := make([]model.Voucher, 0, order.Quantity)
		for i := 0; i < order.Quantity; i++ {
			scanToken, err := generateVoucherScanToken()
			if err != nil {
				return err
			}
			voucher := model.Voucher{
				Code:       generateVoucherCode(),
				ScanToken:  scanToken,
				CouponID:   coupon.ID,
				UserID:     userID,
				OrderID:    &order.ID,
				MerchantID: &merchantID,
				Status:     voucherStatusActive,
				ExpiryDate: coupon.ExpiryDate,
				ValidFrom:  coupon.ValidFrom,
				ValidUntil: coupon.ValidUntil,
			}
			if voucher.ExpiryDate.IsZero() && voucher.ValidUntil != nil {
				voucher.ExpiryDate = *voucher.ValidUntil
			}
			if err := tx.Create(&voucher).Error; err != nil {
				return err
			}
			vouchers = append(vouchers, voucher)
		}
		if err := createOrderPaidNotification(ctx, tx, order); err != nil {
			return err
		}

		result = &PayResult{Order: order, Vouchers: vouchers}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func createOrderPaidNotification(ctx context.Context, tx *gorm.DB, order model.Order) error {
	_, _, err := notificationservice.CreateEventTx(ctx, tx, notificationservice.EventInput{
		RecipientID: order.UserID,
		Type:        model.NotificationTypeOrderPaid,
		Title:       "Payment successful",
		Content:     fmt.Sprintf("Your order #%d has been paid and your voucher is ready.", order.ID),
		Data: map[string]interface{}{
			"order_id": order.ID,
			"status":   order.Status,
		},
		DedupKey: fmt.Sprintf("order_paid:%d", order.ID),
	})
	return err
}

func (s *OrderService) loadPurchasableCoupon(ctx context.Context, couponID int64) (*model.Coupon, *model.Store, error) {
	return s.loadPurchasableCouponTx(s.db.WithContext(ctx), couponID)
}

func (s *OrderService) loadPurchasableCouponTx(db *gorm.DB, couponID int64) (*model.Coupon, *model.Store, error) {
	var coupon model.Coupon
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&coupon, couponID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrCouponNotFound
		}
		return nil, nil, err
	}

	now := time.Now()
	if coupon.Status != couponStatusActive {
		return nil, nil, ErrCouponInactive
	}
	if coupon.StoreID == nil {
		return nil, nil, ErrCouponNotStoreScope
	}
	if coupon.ValidFrom != nil && coupon.ValidFrom.After(now) {
		return nil, nil, ErrCouponNotStarted
	}
	if coupon.ValidUntil != nil && coupon.ValidUntil.Before(now) {
		return nil, nil, ErrCouponExpired
	}
	if !coupon.ExpiryDate.IsZero() && coupon.ExpiryDate.Before(now) {
		return nil, nil, ErrCouponExpired
	}
	if coupon.TotalQuantity <= 0 {
		return nil, nil, ErrCouponSoldOut
	}

	var store model.Store
	if err := db.First(&store, *coupon.StoreID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrStoreNotFound
		}
		return nil, nil, err
	}
	if store.Status != storeStatusPublished {
		return nil, nil, ErrStoreNotPublished
	}
	if store.MerchantID != coupon.MerchantID {
		return nil, nil, ErrCouponStoreMismatch
	}

	return &coupon, &store, nil
}

func generateVoucherCode() string {
	raw := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return "VCH-" + raw[:12]
}

func generateVoucherScanToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate voucher scan token: %w", err)
	}
	return "vst_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
