package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

type CreatePaymentRequest struct {
	OrderID       int64  `json:"order_id" binding:"required"`
	PaymentMethod string `json:"payment_method" binding:"required"`
}

var (
	ErrPaymentInvalidInput     = errors.New("invalid payment input")
	ErrPaymentOrderNotFound    = errors.New("payment order not found")
	ErrPaymentForbidden        = errors.New("payment forbidden")
	ErrPaymentOrderAlreadyPaid = errors.New("payment order already paid")
)

type PaymentService struct {
	db *gorm.DB
}

func NewPaymentService(db *gorm.DB) *PaymentService {
	if db == nil {
		db = database.DB
	}
	return &PaymentService{db: db}
}

func (s *PaymentService) Create(ctx context.Context, userID int64, req CreatePaymentRequest) (model.Payment, error) {
	if userID <= 0 || req.OrderID <= 0 || strings.TrimSpace(req.PaymentMethod) == "" {
		return model.Payment{}, ErrPaymentInvalidInput
	}

	var payment model.Payment
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order model.Order
		if err := tx.Where("id = ? AND user_id = ?", req.OrderID, userID).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPaymentOrderNotFound
			}
			return err
		}
		if order.Status == "paid" {
			return ErrPaymentOrderAlreadyPaid
		}

		// Payment intent creation is idempotent. The amount, currency, order and
		// user are derived from the server-side order, never from the request.
		if err := tx.Where("order_id = ? AND user_id = ?", order.ID, userID).
			Where("status IN ?", []string{"pending", "success"}).
			Order("id DESC").First(&payment).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		userIDCopy := userID
		payment = model.Payment{
			Amount:           order.TotalPrice,
			Currency:         "USD",
			Status:           "pending",
			CouponID:         order.CouponID,
			MerchantID:       order.MerchantID,
			OrderID:          &order.ID,
			UserID:           &userIDCopy,
			PaymentMethod:    strings.TrimSpace(req.PaymentMethod),
			PaymentSessionID: "pay_" + uuid.NewString(),
		}
		return tx.Create(&payment).Error
	})
	return payment, err
}

func (s *PaymentService) Detail(ctx context.Context, userID, id int64) (*model.Payment, error) {
	var p model.Payment
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}
