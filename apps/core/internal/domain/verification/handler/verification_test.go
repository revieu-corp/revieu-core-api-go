package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	if err := db.AutoMigrate(&model.User{}, &model.Merchant{}, &model.MerchantVerification{}); err != nil {
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
}

func TestVerificationHandlerSubmitRequiresActiveMerchantAndValidDocuments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupVerificationTestDB(t)
	users := []model.User{
		{ID: 703, Role: "user", Status: 0},
		{ID: 704, Role: "merchant", Status: 1},
		{ID: 705, Role: "merchant", Status: 0},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("failed to create validation users: %v", err)
	}
	h := NewVerificationHandler(service.NewVerificationService(db))

	submit := func(userID int64, payload string, authenticated bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/merchant/verification", bytes.NewBufferString(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		if authenticated {
			c.Set("user_id", userID)
		}
		h.Submit(c)
		return recorder
	}

	validPayload := `{"document_type":"business_license","document_url":"https://example.com/license.pdf","business_license":"LIC-703"}`
	for _, testCase := range []struct {
		name     string
		userID   int64
		payload  string
		expected int
	}{
		{name: "regular-user", userID: 703, payload: validPayload, expected: http.StatusForbidden},
		{name: "disabled-merchant", userID: 704, payload: validPayload, expected: http.StatusForbidden},
		{name: "missing-document-type", userID: 705, payload: `{"document_type":"","document_url":"https://example.com/license.pdf"}`, expected: http.StatusBadRequest},
		{name: "invalid-document-url", userID: 705, payload: `{"document_type":"business_license","document_url":"javascript:alert(1)"}`, expected: http.StatusBadRequest},
		{name: "oversized-document-type", userID: 705, payload: `{"document_type":"` + strings.Repeat("x", 51) + `","document_url":"https://example.com/license.pdf"}`, expected: http.StatusBadRequest},
		{name: "unauthenticated", userID: 0, payload: validPayload, expected: http.StatusUnauthorized},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			authenticated := testCase.name != "unauthenticated"
			recorder := submit(testCase.userID, testCase.payload, authenticated)
			if recorder.Code != testCase.expected {
				t.Fatalf("expected status %d, got %d: %s", testCase.expected, recorder.Code, recorder.Body.String())
			}
		})
	}

	var merchantCount int64
	if err := db.Model(&model.Merchant{}).Count(&merchantCount).Error; err != nil {
		t.Fatalf("failed to count merchants: %v", err)
	}
	if merchantCount != 0 {
		t.Fatalf("invalid or forbidden submissions created merchants, got %d", merchantCount)
	}
}

func TestVerificationHandlerStatusRejectsNonMerchant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupVerificationTestDB(t)
	if err := db.Create(&model.User{ID: 706, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create regular user: %v", err)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/merchant/verification", nil)
	c.Set("user_id", int64(706))

	NewVerificationHandler(service.NewVerificationService(db)).Status(c)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for regular user, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestVerificationHandlerStatusReturnsUnverifiedWhenNoSubmissionExists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupVerificationTestDB(t)
	if err := db.Create(&model.User{ID: 707, Role: "merchant", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create merchant user: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/merchant/verification", nil)
	c.Set("user_id", int64(707))

	NewVerificationHandler(service.NewVerificationService(db)).Status(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for an active merchant without a submission, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data struct {
			Status         string `json:"status"`
			MerchantStatus string `json:"merchant_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Data.Status != "unverified" || response.Data.MerchantStatus != "unverified" {
		t.Fatalf("expected unverified status pair, got status=%q merchant_status=%q", response.Data.Status, response.Data.MerchantStatus)
	}

	var verificationCount int64
	if err := db.Model(&model.MerchantVerification{}).Count(&verificationCount).Error; err != nil {
		t.Fatalf("failed to count verification records: %v", err)
	}
	if verificationCount != 0 {
		t.Fatalf("status lookup must not create a verification submission, got %d", verificationCount)
	}
}

func TestVerificationHandlerStatusRejectsUnknownUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupVerificationTestDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/merchant/verification", nil)
	c.Set("user_id", int64(9999))

	NewVerificationHandler(service.NewVerificationService(db)).Status(c)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for an unknown user, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var merchantCount int64
	if err := db.Model(&model.Merchant{}).Count(&merchantCount).Error; err != nil {
		t.Fatalf("failed to count merchants: %v", err)
	}
	if merchantCount != 0 {
		t.Fatalf("unknown user status lookup must not create a merchant, got %d", merchantCount)
	}
}
