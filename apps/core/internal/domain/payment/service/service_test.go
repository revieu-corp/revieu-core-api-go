package service

import (
	"context"
	"errors"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
	"gorm.io/gorm"
)

func TestCreatePaymentDerivesOrderFieldsAndScopesDetail(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewPaymentService(db)
	owner := model.User{Role: "user", Status: 0}
	other := model.User{Role: "user", Status: 0}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("failed to create other user: %v", err)
	}
	order := model.Order{UserID: owner.ID, Quantity: 1, TotalPrice: 19.99, Status: "pending"}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	payment, err := svc.Create(context.Background(), owner.ID, CreatePaymentRequest{
		OrderID:       order.ID,
		PaymentMethod: "mock",
	})
	if err != nil {
		t.Fatalf("failed to create payment intent: %v", err)
	}
	if payment.Amount != order.TotalPrice || payment.Currency != "USD" || payment.Status != "pending" {
		t.Fatalf("expected server-derived payment fields, got %+v", payment)
	}
	if payment.UserID == nil || *payment.UserID != owner.ID {
		t.Fatalf("expected payment owner %d, got %+v", owner.ID, payment.UserID)
	}

	repeated, err := svc.Create(context.Background(), owner.ID, CreatePaymentRequest{
		OrderID:       order.ID,
		PaymentMethod: "bank-card",
	})
	if err != nil {
		t.Fatalf("repeated intent creation failed: %v", err)
	}
	if repeated.ID != payment.ID {
		t.Fatalf("expected idempotent payment intent, first=%d repeated=%d", payment.ID, repeated.ID)
	}

	if _, err := svc.Detail(context.Background(), other.ID, payment.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected cross-user payment detail to be hidden, got %v", err)
	}
}

func TestCreatePaymentRejectsMissingOrder(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewPaymentService(db)
	if _, err := svc.Create(context.Background(), 42, CreatePaymentRequest{PaymentMethod: "mock"}); !errors.Is(err, ErrPaymentInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
