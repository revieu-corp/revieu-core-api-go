package service

import (
	"context"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func setupOrderServiceTest(t *testing.T) (*OrderService, *model.User, *model.Merchant, *model.Store, *model.Coupon) {
	t.Helper()

	db := testutil.SetupTestDB(t)
	svc := NewOrderService(db)

	buyer := &model.User{Role: "user", Status: 0}
	if err := db.Create(buyer).Error; err != nil {
		t.Fatalf("failed to create buyer: %v", err)
	}

	merchantOwner := &model.User{Role: "user", Status: 0}
	if err := db.Create(merchantOwner).Error; err != nil {
		t.Fatalf("failed to create merchant owner: %v", err)
	}

	merchant := &model.Merchant{Name: "Scan Token Merchant", UserID: &merchantOwner.ID}
	if err := db.Create(merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	store := &model.Store{
		MerchantID: merchant.ID,
		Name:       "Scan Token Store",
		Status:     storeStatusPublished,
	}
	if err := db.Create(store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	coupon := &model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Scan Token Coupon",
		Type:          "discount",
		Price:         9.99,
		TotalQuantity: 10,
		MaxPerUser:    5,
		Status:        couponStatusActive,
	}
	if err := db.Create(coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	return svc, buyer, merchant, store, coupon
}

func loadVoucherScanTokens(t *testing.T, svc *OrderService, orderID int64) []string {
	t.Helper()

	rows, err := svc.db.Raw("SELECT scan_token FROM vouchers WHERE order_id = ? ORDER BY id ASC", orderID).Rows()
	if err != nil {
		t.Fatalf("failed to query voucher scan tokens: %v", err)
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			t.Fatalf("failed to scan voucher scan token: %v", err)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("voucher scan token rows error: %v", err)
	}

	return tokens
}

func TestPayCreatesVoucherScanTokens(t *testing.T) {
	svc, buyer, _, _, coupon := setupOrderServiceTest(t)

	order, err := svc.Create(context.Background(), buyer.ID, CreateOrderInput{
		CouponID: coupon.ID,
		Quantity: 2,
	})
	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	if _, err := svc.Pay(context.Background(), buyer.ID, order.ID); err != nil {
		t.Fatalf("failed to pay order: %v", err)
	}

	tokens := loadVoucherScanTokens(t, svc, order.ID)
	if len(tokens) != 2 {
		t.Fatalf("expected 2 voucher scan tokens, got %d", len(tokens))
	}
	if tokens[0] == "" || tokens[1] == "" {
		t.Fatalf("expected non-empty voucher scan tokens, got %#v", tokens)
	}
	if tokens[0] == tokens[1] {
		t.Fatalf("expected unique voucher scan tokens, got %#v", tokens)
	}

	var notificationCount int64
	if err := svc.db.Model(&model.Notification{}).
		Where("user_id = ? AND type = ?", buyer.ID, model.NotificationTypeOrderPaid).
		Count(&notificationCount).Error; err != nil {
		t.Fatalf("failed to count order notifications: %v", err)
	}
	if notificationCount != 1 {
		t.Fatalf("expected one order-paid notification, got %d", notificationCount)
	}
}

func TestPayIdempotentKeepsExistingVoucherScanTokens(t *testing.T) {
	svc, buyer, _, _, coupon := setupOrderServiceTest(t)

	order, err := svc.Create(context.Background(), buyer.ID, CreateOrderInput{
		CouponID: coupon.ID,
		Quantity: 2,
	})
	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	if _, err := svc.Pay(context.Background(), buyer.ID, order.ID); err != nil {
		t.Fatalf("failed to pay order first time: %v", err)
	}
	firstTokens := loadVoucherScanTokens(t, svc, order.ID)

	if _, err := svc.Pay(context.Background(), buyer.ID, order.ID); err != nil {
		t.Fatalf("failed to pay order second time: %v", err)
	}
	secondTokens := loadVoucherScanTokens(t, svc, order.ID)

	if len(secondTokens) != 2 {
		t.Fatalf("expected still 2 voucher scan tokens, got %d", len(secondTokens))
	}
	if len(firstTokens) != len(secondTokens) {
		t.Fatalf("expected equal token counts, got first=%d second=%d", len(firstTokens), len(secondTokens))
	}
	for i := range firstTokens {
		if firstTokens[i] != secondTokens[i] {
			t.Fatalf("expected token %d to remain stable, got first=%q second=%q", i, firstTokens[i], secondTokens[i])
		}
	}
	var notificationCount int64
	if err := svc.db.Model(&model.Notification{}).
		Where("user_id = ? AND type = ?", buyer.ID, model.NotificationTypeOrderPaid).
		Count(&notificationCount).Error; err != nil {
		t.Fatalf("failed to count order notifications: %v", err)
	}
	if notificationCount != 1 {
		t.Fatalf("expected idempotent order notification, got %d", notificationCount)
	}
}
