package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/verification/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVerificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Merchant{}, &model.MerchantVerification{}, &model.Notification{}); err != nil {
		t.Fatalf("failed to migrate verification test db: %v", err)
	}

	return db
}

func seedVerificationFixture(t *testing.T, db *gorm.DB) {
	t.Helper()

	user := model.User{ID: 701, Role: "merchant", Status: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	merchant := model.Merchant{ID: 9101, UserID: &user.ID, Name: "Merchant Jane", VerificationStatus: "pending"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	verification := model.MerchantVerification{
		ID:              9201,
		MerchantID:      merchant.ID,
		DocumentType:    "business_license",
		DocumentURL:     "https://example.com/license.pdf",
		BusinessLicense: "LIC-123",
		Status:          "pending",
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now().Add(-time.Hour),
	}
	if err := db.Create(&verification).Error; err != nil {
		t.Fatalf("failed to create verification: %v", err)
	}
}

func TestVerificationHandlerStatusReturnsLatestSubmission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupVerificationTestDB(t)
	seedVerificationFixture(t, db)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/merchant/verification", nil)
	c.Set("user_id", int64(701))

	h := NewVerificationHandler(service.NewVerificationService(db))
	h.Status(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("not implemented")) {
		t.Fatalf("expected verification status payload, got %s", recorder.Body.String())
	}

	var response struct {
		Data struct {
			Status          string `json:"status"`
			DocumentType    string `json:"document_type"`
			BusinessLicense string `json:"business_license"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Data.Status != "pending" {
		t.Fatalf("expected pending status, got %q", response.Data.Status)
	}
	if response.Data.BusinessLicense != "LIC-123" {
		t.Fatalf("expected business license LIC-123, got %q", response.Data.BusinessLicense)
	}
}

func TestVerificationHandlerSubmitCreatesSubmission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupVerificationTestDB(t)

	user := model.User{ID: 702, Role: "merchant", Status: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	body := []byte(`{"document_type":"business_license","document_url":"https://example.com/new-license.pdf","business_license":"LIC-999"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/merchant/verification", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", int64(702))

	h := NewVerificationHandler(service.NewVerificationService(db))
	h.Submit(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", recorder.Code)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("not implemented")) {
		t.Fatalf("expected verification submit payload, got %s", recorder.Body.String())
	}

	var verification model.MerchantVerification
	if err := db.First(&verification, "business_license = ?", "LIC-999").Error; err != nil {
		t.Fatalf("failed to load created verification: %v", err)
	}
	if verification.DocumentURL != "https://example.com/new-license.pdf" {
		t.Fatalf("expected document url to persist, got %q", verification.DocumentURL)
	}
	var notificationCount int64
	if err := db.Model(&model.Notification{}).
		Where("user_id = ? AND type = ?", user.ID, model.NotificationTypeMerchantVerificationChanged).
		Count(&notificationCount).Error; err != nil {
		t.Fatalf("failed to count verification notifications: %v", err)
	}
	if notificationCount != 1 {
		t.Fatalf("expected one verification notification, got %d", notificationCount)
	}
}
