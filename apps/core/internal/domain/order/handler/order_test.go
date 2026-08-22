package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/order/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestPayHandlerForwardsIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	user := &model.User{Role: "user", Status: 0}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	merchantOwner := &model.User{Role: "merchant", Status: 0}
	if err := db.Create(merchantOwner).Error; err != nil {
		t.Fatalf("create merchant owner: %v", err)
	}
	merchant := &model.Merchant{Name: "Payment handler merchant", UserID: &merchantOwner.ID}
	if err := db.Create(merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	store := &model.Store{MerchantID: merchant.ID, Name: "Payment handler store", Status: 1}
	if err := db.Create(store).Error; err != nil {
		t.Fatalf("create store: %v", err)
	}
	coupon := &model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Payment handler coupon",
		Type:          "discount",
		Price:         5,
		TotalQuantity: 5,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(coupon).Error; err != nil {
		t.Fatalf("create coupon: %v", err)
	}

	svc := service.NewOrderServiceWithMockPayments(db, true)
	order, err := svc.Create(httptest.NewRequest(http.MethodPost, "/orders", nil).Context(), user.ID, service.CreateOrderInput{CouponID: coupon.ID, Quantity: 1})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	h := NewOrderHandler(svc)

	call := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/orders/"+toString(order.ID)+"/pay", nil)
		c.Request.Header.Set("Idempotency-Key", "handler-checkout-1")
		c.Params = gin.Params{{Key: "id", Value: toString(order.ID)}}
		c.Set("user_id", user.ID)
		h.Pay(c)
		return recorder
	}

	if recorder := call(); recorder.Code != http.StatusOK {
		t.Fatalf("first pay status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := call(); recorder.Code != http.StatusOK {
		t.Fatalf("idempotent replay status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var attemptCount int64
	if err := db.Model(&model.PaymentAttempt{}).Where("order_id = ?", order.ID).Count(&attemptCount).Error; err != nil {
		t.Fatalf("count payment attempts: %v", err)
	}
	if attemptCount != 1 {
		t.Fatalf("expected one payment attempt, got %d", attemptCount)
	}
}

func toString(value int64) string {
	return fmt.Sprintf("%d", value)
}
