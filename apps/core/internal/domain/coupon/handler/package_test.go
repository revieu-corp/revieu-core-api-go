package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/coupon/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPackageHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Package{}, &model.Coupon{}); err != nil {
		t.Fatalf("failed to migrate package test db: %v", err)
	}
	return db
}

func TestCouponHandlerListPackagesReturnsActivePackagesAndCoupons(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPackageHandlerDB(t)

	activePackage := model.Package{ID: 1301, MerchantID: 1, Title: "Active bundle", Status: "active"}
	inactivePackage := model.Package{ID: 1302, MerchantID: 1, Title: "Hidden bundle", Status: "inactive"}
	if err := db.Create(&activePackage).Error; err != nil {
		t.Fatalf("failed to create active package: %v", err)
	}
	if err := db.Create(&inactivePackage).Error; err != nil {
		t.Fatalf("failed to create inactive package: %v", err)
	}
	activeCoupon := model.Coupon{ID: 1311, MerchantID: 1, PackageID: &activePackage.ID, Title: "Included coupon", Type: "cash", Status: "active"}
	inactiveCoupon := model.Coupon{ID: 1312, MerchantID: 1, PackageID: &activePackage.ID, Title: "Hidden coupon", Type: "cash", Status: "inactive"}
	if err := db.Create(&activeCoupon).Error; err != nil {
		t.Fatalf("failed to create active coupon: %v", err)
	}
	if err := db.Create(&inactiveCoupon).Error; err != nil {
		t.Fatalf("failed to create inactive coupon: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/packages?limit=1", nil)
	NewCouponHandler(service.NewCouponService(db)).ListPackages(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response service.PackagePage
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode package page: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].ID != activePackage.ID {
		t.Fatalf("expected only active package, got %+v", response.Data)
	}
	if len(response.Data[0].Coupons) != 1 || response.Data[0].Coupons[0].ID != activeCoupon.ID {
		t.Fatalf("expected only active included coupon, got %+v", response.Data[0].Coupons)
	}
	if response.Cursor != nil {
		t.Fatalf("expected no cursor for one active package, got %v", *response.Cursor)
	}
}

func TestCouponHandlerListPackagesReturnsEmptyArrayAndRejectsInvalidQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPackageHandlerDB(t)
	h := NewCouponHandler(service.NewCouponService(db))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/packages", nil)
	h.ListPackages(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for an empty package list, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response service.PackagePage
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode empty package page: %v", err)
	}
	if response.Data == nil || len(response.Data) != 0 {
		t.Fatalf("expected an empty data array, got %#v", response.Data)
	}

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/packages?limit=101", nil)
	h.ListPackages(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for an oversized page, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestCouponHandlerPackageDetailReturnsActivePackageAndMapsMissingResources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPackageHandlerDB(t)
	activePackage := model.Package{ID: 1401, MerchantID: 1, Title: "Active detail", Status: "active"}
	inactivePackage := model.Package{ID: 1402, MerchantID: 1, Title: "Inactive detail", Status: "inactive"}
	if err := db.Create(&activePackage).Error; err != nil {
		t.Fatalf("failed to create active package: %v", err)
	}
	if err := db.Create(&inactivePackage).Error; err != nil {
		t.Fatalf("failed to create inactive package: %v", err)
	}
	coupon := model.Coupon{ID: 1411, MerchantID: 1, PackageID: &activePackage.ID, Title: "Detail coupon", Type: "cash", Status: "active"}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create detail coupon: %v", err)
	}
	h := NewCouponHandler(service.NewCouponService(db))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/packages/1401", nil)
	c.Params = gin.Params{{Key: "id", Value: "1401"}}
	h.PackageDetail(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response service.PackageResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode package detail: %v", err)
	}
	if response.Data == nil || response.Data.ID != activePackage.ID || len(response.Data.Coupons) != 1 {
		t.Fatalf("unexpected package detail: %+v", response.Data)
	}

	for _, id := range []string{"1402", "9999"} {
		recorder = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/packages/"+id, nil)
		c.Params = gin.Params{{Key: "id", Value: id}}
		h.PackageDetail(c)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected status 404 for package %s, got %d: %s", id, recorder.Code, recorder.Body.String())
		}
	}

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/packages/nope", nil)
	c.Params = gin.Params{{Key: "id", Value: "nope"}}
	h.PackageDetail(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for an invalid package ID, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
