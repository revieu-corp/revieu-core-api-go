package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/admin/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Report{}, &model.AdminAuditLog{}, &model.Merchant{}, &model.Notification{}); err != nil {
		t.Fatalf("failed to migrate admin test db: %v", err)
	}
	return db
}

func adminContext(method, path, body string, userID int64, role string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	if userID > 0 {
		c.Set("user_id", userID)
	}
	if role != "" {
		c.Set("user_role", role)
	}
	return c, recorder
}

func TestAdminHandlerListReportsFiltersAndPaginates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminTestDB(t)
	now := time.Now().UTC()
	reports := []model.Report{
		{ID: 1001, ReporterID: 1, TargetType: "review", TargetID: 11, Reason: "spam", Status: "pending", CreatedAt: now.Add(-time.Minute)},
		{ID: 1002, ReporterID: 2, TargetType: "post", TargetID: 12, Reason: "fake", Status: "resolved", CreatedAt: now.Add(-2 * time.Minute)},
	}
	if err := db.Create(&reports).Error; err != nil {
		t.Fatalf("failed to seed reports: %v", err)
	}

	c, recorder := adminContext(http.MethodGet, "/admin/reports?status=pending&limit=1", "", 9001, "admin")
	NewAdminHandler(service.NewAdminService(db)).ListReports(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response service.ReportPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].ID != 1001 {
		t.Fatalf("expected one pending report, got %+v", response.Data)
	}
	if response.Cursor != nil {
		t.Fatalf("expected no cursor when the filtered result fits one page, got %v", *response.Cursor)
	}
}

func TestAdminHandlerListReportsRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminTestDB(t)
	h := NewAdminHandler(service.NewAdminService(db))

	tests := []struct {
		name     string
		userID   int64
		role     string
		expected int
	}{
		{name: "unauthenticated", expected: http.StatusUnauthorized},
		{name: "regular-user", userID: 9002, role: "user", expected: http.StatusForbidden},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			c, recorder := adminContext(http.MethodGet, "/admin/reports", "", testCase.userID, testCase.role)
			h.ListReports(c)
			if recorder.Code != testCase.expected {
				t.Fatalf("expected status %d, got %d: %s", testCase.expected, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAdminHandlerUpdateReportWritesAuditLogAndRejectsInvalidState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminTestDB(t)
	if err := db.Create(&model.Report{ID: 1101, ReporterID: 1, TargetType: "review", TargetID: 11, Reason: "spam", Status: "pending"}).Error; err != nil {
		t.Fatalf("failed to seed report: %v", err)
	}
	h := NewAdminHandler(service.NewAdminService(db))

	c, recorder := adminContext(http.MethodPatch, "/admin/reports/1101", `{"status":"resolved","resolution":"removed spam"}`, 9003, "admin")
	c.Params = gin.Params{{Key: "id", Value: "1101"}}
	h.UpdateReport(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var report model.Report
	if err := db.First(&report, 1101).Error; err != nil {
		t.Fatalf("failed to load updated report: %v", err)
	}
	if report.Status != "resolved" || report.ReviewedBy == nil || *report.ReviewedBy != 9003 || report.ReviewedAt == nil {
		t.Fatalf("updated report did not persist review metadata: %+v", report)
	}
	var audit model.AdminAuditLog
	if err := db.Where("target_type = ? AND target_id = ?", "report", 1101).First(&audit).Error; err != nil {
		t.Fatalf("failed to load report audit log: %v", err)
	}
	if audit.AdminID != 9003 || audit.Action != "resolve_report" {
		t.Fatalf("unexpected report audit log: %+v", audit)
	}

	c, recorder = adminContext(http.MethodPatch, "/admin/reports/1101", `{"status":"dismissed","resolution":"duplicate"}`, 9003, "admin")
	c.Params = gin.Params{{Key: "id", Value: "1101"}}
	h.UpdateReport(c)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status 409 for a terminal report, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminHandlerUpdateReportValidatesAndMapsMissingResources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminTestDB(t)
	h := NewAdminHandler(service.NewAdminService(db))

	c, recorder := adminContext(http.MethodPatch, "/admin/reports/9999", `{"status":"resolved","resolution":"missing"}`, 9004, "admin")
	c.Params = gin.Params{{Key: "id", Value: "9999"}}
	h.UpdateReport(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for missing report, got %d: %s", recorder.Code, recorder.Body.String())
	}

	c, recorder = adminContext(http.MethodPatch, "/admin/reports/9999", `{"status":"unknown"}`, 9004, "admin")
	c.Params = gin.Params{{Key: "id", Value: "9999"}}
	h.UpdateReport(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid report status, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminHandlerListAndUpdateMerchants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminTestDB(t)
	if err := db.Create(&[]model.Merchant{
		{ID: 1201, Name: "Verified Cafe", VerificationStatus: "verified", Status: 0},
		{ID: 1202, Name: "Pending Cafe", VerificationStatus: "pending", Status: 0},
	}).Error; err != nil {
		t.Fatalf("failed to seed merchants: %v", err)
	}
	h := NewAdminHandler(service.NewAdminService(db))

	c, recorder := adminContext(http.MethodGet, "/admin/merchants?verification_status=verified", "", 9005, "admin")
	h.ListMerchants(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var page service.MerchantPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("failed to decode merchant page: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != 1201 {
		t.Fatalf("expected only verified merchant, got %+v", page.Data)
	}

	c, recorder = adminContext(http.MethodPatch, "/admin/merchants/1202", `{"verification_status":"verified"}`, 9005, "admin")
	c.Params = gin.Params{{Key: "id", Value: "1202"}}
	h.UpdateMerchant(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for merchant update, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var merchant model.Merchant
	if err := db.First(&merchant, 1202).Error; err != nil {
		t.Fatalf("failed to load updated merchant: %v", err)
	}
	if merchant.VerificationStatus != "verified" || merchant.VerifiedAt == nil {
		t.Fatalf("merchant verification update did not set verified state: %+v", merchant)
	}
	var audit model.AdminAuditLog
	if err := db.Where("target_type = ? AND target_id = ?", "merchant", 1202).First(&audit).Error; err != nil {
		t.Fatalf("failed to load merchant audit log: %v", err)
	}
	if audit.AdminID != 9005 || audit.Action != "update_merchant" {
		t.Fatalf("unexpected merchant audit log: %+v", audit)
	}
}

func TestAdminHandlerMerchantErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminTestDB(t)
	h := NewAdminHandler(service.NewAdminService(db))

	c, recorder := adminContext(http.MethodGet, "/admin/merchants?status=bad", "", 9006, "admin")
	h.ListMerchants(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid merchant query, got %d: %s", recorder.Code, recorder.Body.String())
	}

	c, recorder = adminContext(http.MethodPatch, "/admin/merchants/9999", `{}`, 9006, "admin")
	c.Params = gin.Params{{Key: "id", Value: "9999"}}
	h.UpdateMerchant(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty merchant update, got %d: %s", recorder.Code, recorder.Body.String())
	}

	c, recorder = adminContext(http.MethodPatch, "/admin/merchants/9999", `{"status":0}`, 9006, "admin")
	c.Params = gin.Params{{Key: "id", Value: "9999"}}
	h.UpdateMerchant(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for missing merchant, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
