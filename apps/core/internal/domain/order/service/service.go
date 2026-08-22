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
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	storeStatusPublished           int16 = 1
	couponStatusActive                   = "active"
	orderStatusPending                   = "pending"
	paymentAttemptStatusPending          = "pending"
	paymentAttemptStatusProcessing       = "processing"
	paymentAttemptStatusPaid             = "paid"
	paymentAttemptStatusFailed           = "failed"
	orderStatusPaid                      = "paid"
	voucherStatusActive                  = "active"
	paymentStatusPending                 = "pending"
	paymentStatusProcessing              = "processing"
	paymentStatusSuccess                 = "success"
)

var (
	ErrOrderNotFound              = errors.New("order not found")
	ErrOrderForbidden             = errors.New("order forbidden")
	ErrOrderInvalidInput          = errors.New("invalid order input")
	ErrOrderInvalidState          = errors.New("invalid order state")
	ErrPaymentProviderUnavailable = errors.New("payment provider unavailable")
	ErrPaymentInProgress          = errors.New("payment already in progress")
	ErrPaymentAttemptNotFound     = errors.New("payment attempt not found")
	ErrPaymentAttemptInvalidState = errors.New("invalid payment attempt state")
	ErrStoreNotFound              = errors.New("store not found")
	ErrStoreNotPublished          = errors.New("store not published")
	ErrCouponNotFound             = errors.New("coupon not found")
	ErrCouponInactive             = errors.New("coupon inactive")
	ErrCouponNotStarted           = errors.New("coupon not started")
	ErrCouponExpired              = errors.New("coupon expired")
	ErrCouponSoldOut              = errors.New("coupon sold out")
	ErrCouponNotStoreScope        = errors.New("coupon must be store scoped")
	ErrCouponStoreMismatch        = errors.New("coupon store mismatch")
	ErrCouponPerUserLimit         = errors.New("coupon per-user limit exceeded")
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
	Order            model.Order     `json:"order"`
	Vouchers         []model.Voucher `json:"vouchers"`
	PaymentAttemptID int64           `json:"payment_attempt_id"`
	PaymentStatus    string          `json:"payment_status"`
}

type PaymentCallbackInput struct {
	Status            string
	ProviderReference string
	FailureReason     string
}

type OrderService struct {
	db                *gorm.DB
	allowMockPayments bool
}

func NewOrderService(db *gorm.DB) *OrderService {
	return NewOrderServiceWithMockPayments(db, false)
}

func NewOrderServiceWithMockPayments(db *gorm.DB, allowMockPayments bool) *OrderService {
	if db == nil {
		db = database.DB
	}
	return &OrderService{db: db, allowMockPayments: allowMockPayments}
}

func NewOrderServiceForMode(db *gorm.DB, mode string) *OrderService {
	mode = strings.ToLower(strings.TrimSpace(mode))
	allowMockPayments := mode == "debug" || mode == "development" || mode == "dev" || mode == "test"
	return NewOrderServiceWithMockPayments(db, allowMockPayments)
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
	return s.PayWithIdempotencyKey(ctx, userID, orderID, "")
}

// PayWithIdempotencyKey executes one durable payment attempt. An empty key is
// replaced with a deterministic per-order key for backwards compatibility
// with existing clients that do not yet send Idempotency-Key.
func (s *OrderService) PayWithIdempotencyKey(ctx context.Context, userID, orderID int64, idempotencyKey string) (*PayResult, error) {
	if userID <= 0 || orderID <= 0 {
		return nil, ErrOrderInvalidInput
	}
	key, err := normalizeIdempotencyKey(orderID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	attempt, replay, err := s.beginPaymentAttempt(ctx, userID, orderID, key)
	if err != nil {
		return nil, err
	}
	if replay {
		return s.resultForPaidAttempt(ctx, attempt)
	}

	result, err := s.settlePaymentAttempt(ctx, attempt.ID, false)
	if err != nil {
		_ = s.markPaymentAttemptFailed(ctx, attempt.ID, err.Error())
		return nil, err
	}
	return result, nil
}

func normalizeIdempotencyKey(orderID int64, raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return fmt.Sprintf("order:%d:default", orderID), nil
	}
	if len(key) > 255 {
		return "", ErrOrderInvalidInput
	}
	return key, nil
}

func (s *OrderService) beginPaymentAttempt(ctx context.Context, userID, orderID int64, key string) (*model.PaymentAttempt, bool, error) {
	var attempt *model.PaymentAttempt
	replay := false
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if order.Status == orderStatusPaid {
			var paidAttempt model.PaymentAttempt
			if err := tx.Where("order_id = ? AND user_id = ? AND status = ?", orderID, userID, paymentAttemptStatusPaid).Order("id DESC").First(&paidAttempt).Error; err == nil {
				attempt = &paidAttempt
				replay = true
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		var activeAttempt model.PaymentAttempt
		if err := tx.Where("order_id = ? AND user_id = ? AND status = ?", orderID, userID, paymentAttemptStatusProcessing).Order("id DESC").First(&activeAttempt).Error; err == nil {
			if activeAttempt.IdempotencyKey != key {
				return ErrPaymentInProgress
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var existing model.PaymentAttempt
		lookupErr := tx.Where("order_id = ? AND idempotency_key = ?", orderID, key).First(&existing).Error
		if lookupErr == nil {
			switch existing.Status {
			case paymentAttemptStatusPaid:
				attempt = &existing
				replay = true
				return nil
			case paymentAttemptStatusProcessing:
				return ErrPaymentInProgress
			case paymentAttemptStatusPending, paymentAttemptStatusFailed:
				if err := tx.Model(&model.PaymentAttempt{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
					"status":             paymentAttemptStatusProcessing,
					"failure_reason":     "",
					"provider_reference": "",
					"started_at":         now,
					"completed_at":       nil,
				}).Error; err != nil {
					return err
				}
				existing.Status = paymentAttemptStatusProcessing
				existing.FailureReason = ""
				existing.ProviderReference = ""
				existing.StartedAt = now
				existing.CompletedAt = nil
				attempt = &existing
				return nil
			default:
				return ErrPaymentAttemptInvalidState
			}
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}

		candidate := model.PaymentAttempt{
			OrderID:        orderID,
			UserID:         userID,
			IdempotencyKey: key,
			Status:         paymentAttemptStatusProcessing,
			Amount:         order.TotalPrice,
			Currency:       "USD",
			PaymentMethod:  "mock",
			StartedAt:      now,
		}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 1 {
			attempt = &candidate
			return nil
		}

		if err := tx.Where("order_id = ? AND idempotency_key = ?", orderID, key).First(&existing).Error; err != nil {
			return err
		}
		if existing.Status == paymentAttemptStatusPaid {
			attempt = &existing
			replay = true
			return nil
		}
		if existing.Status == paymentAttemptStatusProcessing {
			return ErrPaymentInProgress
		}
		if existing.Status != paymentAttemptStatusPending && existing.Status != paymentAttemptStatusFailed {
			return ErrPaymentAttemptInvalidState
		}
		if err := tx.Model(&model.PaymentAttempt{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"status":             paymentAttemptStatusProcessing,
			"failure_reason":     "",
			"provider_reference": "",
			"started_at":         now,
			"completed_at":       nil,
		}).Error; err != nil {
			return err
		}
		existing.Status = paymentAttemptStatusProcessing
		existing.FailureReason = ""
		existing.ProviderReference = ""
		existing.StartedAt = now
		existing.CompletedAt = nil
		attempt = &existing
		return nil
	})
	return attempt, replay, err
}

func (s *OrderService) resultForPaidAttempt(ctx context.Context, attempt *model.PaymentAttempt) (*PayResult, error) {
	var order model.Order
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", attempt.OrderID, attempt.UserID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	var vouchers []model.Voucher
	if err := s.db.WithContext(ctx).Where("order_id = ?", order.ID).Order("id ASC").Find(&vouchers).Error; err != nil {
		return nil, err
	}
	return &PayResult{Order: order, Vouchers: vouchers, PaymentAttemptID: attempt.ID, PaymentStatus: attempt.Status}, nil
}

func (s *OrderService) settlePaymentAttempt(ctx context.Context, attemptID int64, providerConfirmed bool) (*PayResult, error) {
	var result *PayResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt model.PaymentAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&attempt, attemptID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPaymentAttemptNotFound
			}
			return err
		}
		if attempt.Status == paymentAttemptStatusPaid {
			var order model.Order
			if err := tx.Where("id = ? AND user_id = ?", attempt.OrderID, attempt.UserID).First(&order).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrOrderNotFound
				}
				return err
			}
			var vouchers []model.Voucher
			if err := tx.Where("order_id = ?", order.ID).Order("id ASC").Find(&vouchers).Error; err != nil {
				return err
			}
			result = &PayResult{Order: order, Vouchers: vouchers, PaymentAttemptID: attempt.ID, PaymentStatus: attempt.Status}
			return nil
		}
		if attempt.Status != paymentAttemptStatusProcessing {
			return ErrPaymentAttemptInvalidState
		}
		if !providerConfirmed && !s.allowMockPayments {
			return ErrPaymentProviderUnavailable
		}

		var order model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, attempt.OrderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}
		if order.UserID != attempt.UserID {
			return ErrOrderForbidden
		}
		if order.Status == orderStatusPaid {
			var vouchers []model.Voucher
			if err := tx.Where("order_id = ?", order.ID).Order("id ASC").Find(&vouchers).Error; err != nil {
				return err
			}
			now := time.Now().UTC()
			if err := tx.Model(&model.PaymentAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]interface{}{
				"status":       paymentAttemptStatusPaid,
				"completed_at": now,
			}).Error; err != nil {
				return err
			}
			result = &PayResult{Order: order, Vouchers: vouchers, PaymentAttemptID: attempt.ID, PaymentStatus: paymentAttemptStatusPaid}
			return nil
		}
		if order.Status != orderStatusPending {
			return ErrOrderInvalidState
		}
		if order.CouponID == nil || order.Quantity <= 0 {
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
				Where("user_id = ? AND coupon_id = ?", attempt.UserID, coupon.ID).
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

		now := time.Now().UTC()
		merchantID := coupon.MerchantID
		userIDCopy := attempt.UserID
		paymentSessionID := attempt.ProviderReference
		if paymentSessionID == "" {
			paymentSessionID = fmt.Sprintf("mock-order-%d", order.ID)
		}
		var payment model.Payment
		paymentErr := tx.Where("order_id = ? AND user_id = ? AND status IN ?", order.ID, attempt.UserID, []string{paymentStatusPending, paymentStatusProcessing}).Order("id DESC").First(&payment).Error
		switch {
		case paymentErr == nil:
			if err := tx.Model(&payment).Updates(map[string]interface{}{
				"amount":             order.TotalPrice,
				"currency":           attempt.Currency,
				"status":             paymentStatusSuccess,
				"coupon_id":          order.CouponID,
				"merchant_id":        &merchantID,
				"order_id":           &order.ID,
				"user_id":            &userIDCopy,
				"payment_method":     attempt.PaymentMethod,
				"payment_session_id": paymentSessionID,
				"paid_at":            &now,
			}).Error; err != nil {
				return err
			}
		case errors.Is(paymentErr, gorm.ErrRecordNotFound):
			payment = model.Payment{
				Amount:           order.TotalPrice,
				Currency:         attempt.Currency,
				Status:           paymentStatusSuccess,
				CouponID:         order.CouponID,
				MerchantID:       &merchantID,
				OrderID:          &order.ID,
				UserID:           &userIDCopy,
				PaymentMethod:    attempt.PaymentMethod,
				PaymentSessionID: paymentSessionID,
				PaidAt:           &now,
			}
			if err := tx.Create(&payment).Error; err != nil {
				return err
			}
		default:
			return paymentErr
		}

		if err := tx.Model(&model.Order{}).Where("id = ?", order.ID).UpdateColumn("status", orderStatusPaid).Error; err != nil {
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
				UserID:     attempt.UserID,
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

		if err := tx.Model(&model.PaymentAttempt{}).Where("id = ? AND status = ?", attempt.ID, paymentAttemptStatusProcessing).Updates(map[string]interface{}{
			"status":       paymentAttemptStatusPaid,
			"payment_id":   payment.ID,
			"completed_at": now,
		}).Error; err != nil {
			return err
		}
		result = &PayResult{Order: order, Vouchers: vouchers, PaymentAttemptID: attempt.ID, PaymentStatus: paymentAttemptStatusPaid}
		return nil
	})
	return result, err
}

// ApplyPaymentCallback is the gateway-agnostic settlement hook for a trusted
// provider integration. It is intentionally a service contract rather than a
// public unauthenticated HTTP endpoint; the integration owns authentication.
func (s *OrderService) ApplyPaymentCallback(ctx context.Context, attemptID int64, input PaymentCallbackInput) (*PayResult, error) {
	if attemptID <= 0 {
		return nil, ErrOrderInvalidInput
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	switch status {
	case paymentAttemptStatusProcessing:
		return nil, s.markPaymentAttemptProcessing(ctx, attemptID, strings.TrimSpace(input.ProviderReference))
	case paymentAttemptStatusFailed:
		reason := strings.TrimSpace(input.FailureReason)
		if reason == "" {
			reason = "gateway payment failed"
		}
		return nil, s.markPaymentAttemptFailed(ctx, attemptID, reason)
	case paymentAttemptStatusPaid:
		if err := s.markPaymentAttemptProcessing(ctx, attemptID, strings.TrimSpace(input.ProviderReference)); err != nil {
			return nil, err
		}
		result, err := s.settlePaymentAttempt(ctx, attemptID, true)
		if err != nil {
			_ = s.markPaymentAttemptFailed(ctx, attemptID, err.Error())
		}
		return result, err
	default:
		return nil, ErrOrderInvalidInput
	}
}

func (s *OrderService) markPaymentAttemptProcessing(ctx context.Context, attemptID int64, providerReference string) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt model.PaymentAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&attempt, attemptID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPaymentAttemptNotFound
			}
			return err
		}
		if attempt.Status == paymentAttemptStatusPaid {
			return nil
		}
		if attempt.Status == paymentAttemptStatusFailed {
			return ErrPaymentAttemptInvalidState
		}
		updates := map[string]interface{}{"status": paymentAttemptStatusProcessing, "started_at": now}
		if providerReference != "" {
			updates["provider_reference"] = providerReference
		}
		return tx.Model(&model.PaymentAttempt{}).Where("id = ?", attempt.ID).Updates(updates).Error
	})
}

func (s *OrderService) markPaymentAttemptFailed(ctx context.Context, attemptID int64, reason string) error {
	now := time.Now().UTC()
	reason = truncateFailureReason(strings.TrimSpace(reason))
	if reason == "" {
		reason = "payment failed"
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt model.PaymentAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&attempt, attemptID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPaymentAttemptNotFound
			}
			return err
		}
		switch attempt.Status {
		case paymentAttemptStatusFailed:
			return nil
		case paymentAttemptStatusPaid:
			return ErrPaymentAttemptInvalidState
		case paymentAttemptStatusPending, paymentAttemptStatusProcessing:
			return tx.Model(&model.PaymentAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]interface{}{
				"status":         paymentAttemptStatusFailed,
				"failure_reason": reason,
				"completed_at":   now,
			}).Error
		default:
			return ErrPaymentAttemptInvalidState
		}
	})
}

func truncateFailureReason(reason string) string {
	const maxFailureReason = 1000
	if len(reason) <= maxFailureReason {
		return reason
	}
	return reason[:maxFailureReason]
}

// ReconcileStalePaymentAttempts marks abandoned processing attempts as failed
// and returns the number of attempts transitioned. Callers should schedule it
// periodically with a timeout appropriate to the configured gateway.
func (s *OrderService) ReconcileStalePaymentAttempts(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, ErrOrderInvalidInput
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	now := time.Now().UTC()
	var attempts []model.PaymentAttempt
	if err := s.db.WithContext(ctx).
		Where("status = ? AND started_at < ?", paymentAttemptStatusProcessing, cutoff).
		Order("id ASC").Find(&attempts).Error; err != nil {
		return 0, err
	}
	var transitioned int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, attempt := range attempts {
			result := tx.Model(&model.PaymentAttempt{}).Where("id = ? AND status = ? AND started_at < ?", attempt.ID, paymentAttemptStatusProcessing, cutoff).Updates(map[string]interface{}{
				"status":         paymentAttemptStatusFailed,
				"failure_reason": "stale_processing_timeout",
				"completed_at":   now,
			})
			if result.Error != nil {
				return result.Error
			}
			transitioned += result.RowsAffected
		}
		return nil
	})
	return transitioned, err
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
