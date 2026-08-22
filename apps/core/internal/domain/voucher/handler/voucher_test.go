package handler

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/voucher/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeleteArchivesAuthenticatedUsersVoucher(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Voucher{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	if err := db.Create(&model.User{ID: 2201, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	voucher := model.Voucher{
		Code:      "HANDLER-DELETE",
		ScanToken: "HANDLER-DELETE-TOKEN",
		UserID:    2201,
		Status:    "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/vouchers/"+strconv.FormatInt(voucher.ID, 10), nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(voucher.ID, 10)}}
	ctx.Set("user_id", int64(2201))

	NewVoucherHandler(service.NewVoucherService(db), "").Delete(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var archived model.Voucher
	if err := db.First(&archived, voucher.ID).Error; err != nil {
		t.Fatalf("failed to reload voucher: %v", err)
	}
	if archived.Status != "archived" {
		t.Fatalf("expected archived status, got %q", archived.Status)
	}
}
