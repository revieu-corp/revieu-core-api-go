package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/coupon/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMerchantCouponHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Merchant{}, &model.Store{}, &model.Coupon{}); err != nil {
		t.Fatalf("failed to migrate merchant coupon test db: %v", err)
	}
	return db
}

func merchantCouponContext(method, path, body string, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	if userID > 0 {
		c.Set("user_id", userID)
	}
	return c, recorder
}

func TestCouponHandlerMerchantLifecycleEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMerchantCouponHandlerDB(t)
	userID := int64(1601)
	if err := db.Create(&model.User{ID: userID, Role: "merchant", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create merchant user: %v", err)
	}
	merchant := model.Merchant{UserID: &userID, Name: "Handler merchant", VerificationStatus: "verified"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Handler store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	coupon := model.Coupon{MerchantID: merchant.ID, StoreID: &store.ID, Title: "Handler coupon", Type: "cash", TotalQuantity: 10, MaxPerUser: 1, Status: "draft"}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	h := NewCouponHandler(service.NewCouponService(db))

	c, recorder := merchantCouponContext(http.MethodGet, "/merchant/coupons?status=draft&limit=1", "", userID)
	h.ListMerchantCoupons(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var page service.CouponPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("failed to decode merchant coupon page: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != coupon.ID {
		t.Fatalf("unexpected merchant coupon page: %+v", page)
	}

	c, recorder = merchantCouponContext(http.MethodPatch, "/merchant/coupons/"+toString(coupon.ID), `{"title":"Updated handler coupon"}`, userID)
	c.Params = gin.Params{{Key: "id", Value: toString(coupon.ID)}}
	h.UpdateMerchantCoupon(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected update status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	c, recorder = merchantCouponContext(http.MethodPost, "/merchant/coupons/"+toString(coupon.ID)+"/activate", "", userID)
	c.Params = gin.Params{{Key: "id", Value: toString(coupon.ID)}}
	h.ActivateMerchantCoupon(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected activate status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	c, recorder = merchantCouponContext(http.MethodPost, "/merchant/coupons/"+toString(coupon.ID)+"/deactivate", "", userID)
	c.Params = gin.Params{{Key: "id", Value: toString(coupon.ID)}}
	h.DeactivateMerchantCoupon(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected deactivate status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestCouponHandlerMerchantLifecycleAuthErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMerchantCouponHandlerDB(t)
	h := NewCouponHandler(service.NewCouponService(db))

	c, recorder := merchantCouponContext(http.MethodGet, "/merchant/coupons", "", 0)
	h.ListMerchantCoupons(c)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated list status 401, got %d: %s", recorder.Code, recorder.Body.String())
	}

	regularID := int64(1602)
	if err := db.Create(&model.User{ID: regularID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create regular user: %v", err)
	}
	c, recorder = merchantCouponContext(http.MethodGet, "/merchant/coupons", "", regularID)
	h.ListMerchantCoupons(c)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected regular user list status 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func toString(value int64) string {
	return fmt.Sprintf("%d", value)
}
