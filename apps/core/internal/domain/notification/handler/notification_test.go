package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/notification/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNotificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Notification{}); err != nil {
		t.Fatalf("failed to migrate notification test db: %v", err)
	}

	return db
}

func seedNotificationFixture(t *testing.T, db *gorm.DB) {
	t.Helper()

	user := model.User{ID: 601, Role: "merchant", Status: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	notifications := []model.Notification{
		{
			ID:        8001,
			UserID:    user.ID,
			Type:      "verification",
			Title:     "Verification submitted",
			Content:   "We received your business documents.",
			IsRead:    false,
			CreatedAt: time.Now().Add(-time.Hour),
		},
	}
	if err := db.Create(&notifications).Error; err != nil {
		t.Fatalf("failed to create notifications: %v", err)
	}
}

func TestNotificationHandlerListReturnsNotifications(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupNotificationTestDB(t)
	seedNotificationFixture(t, db)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/notifications", nil)
	c.Set("user_id", int64(601))

	h := NewNotificationHandler(service.NewNotificationService(db))
	h.List(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("not implemented")) {
		t.Fatalf("expected notification list payload, got %s", recorder.Body.String())
	}

	var response struct {
		Data []struct {
			ID      int64  `json:"id"`
			Title   string `json:"title"`
			Content string `json:"content"`
			IsRead  bool   `json:"is_read"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(response.Data) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(response.Data))
	}
	if response.Data[0].Title != "Verification submitted" {
		t.Fatalf("expected verification notification title, got %q", response.Data[0].Title)
	}
}

func TestNotificationHandlerListSupportsPaginationAndUserIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupNotificationTestDB(t)
	seedNotificationFixture(t, db)
	if err := db.Create(&model.User{ID: 602, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create unrelated user: %v", err)
	}
	if err := db.Create(&model.Notification{
		ID:        8002,
		UserID:    601,
		Type:      "review",
		Title:     "New review",
		Content:   "A customer left a review.",
		IsRead:    true,
		CreatedAt: time.Now().Add(-30 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("failed to create second notification: %v", err)
	}
	if err := db.Create(&model.Notification{
		ID:        8003,
		UserID:    602,
		Type:      "private",
		Title:     "Private notification",
		CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("failed to create unrelated notification: %v", err)
	}

	h := NewNotificationHandler(service.NewNotificationService(db))
	request := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		c.Set("user_id", int64(601))
		h.List(c)
		return recorder
	}

	firstRecorder := request("/notifications?limit=1")
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("expected first page status 200, got %d: %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	var firstPage struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
		Cursor *int64 `json:"cursor"`
	}
	if err := json.Unmarshal(firstRecorder.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("failed to decode first notification page: %v", err)
	}
	if len(firstPage.Data) != 1 || firstPage.Data[0].ID != 8002 {
		t.Fatalf("expected newest notification 8002, got %+v", firstPage.Data)
	}
	if firstPage.Cursor == nil || *firstPage.Cursor != 8002 {
		t.Fatalf("expected cursor 8002, got %v", firstPage.Cursor)
	}

	secondRecorder := request("/notifications?limit=1&cursor=8002")
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("expected second page status 200, got %d: %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	var secondPage struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
		Cursor *int64 `json:"cursor"`
	}
	if err := json.Unmarshal(secondRecorder.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("failed to decode second notification page: %v", err)
	}
	if len(secondPage.Data) != 1 || secondPage.Data[0].ID != 8001 || secondPage.Cursor != nil {
		t.Fatalf("expected oldest notification 8001 without cursor, got %+v cursor=%v", secondPage.Data, secondPage.Cursor)
	}
}

func TestNotificationHandlerListRejectsUnauthenticatedAndInvalidPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupNotificationTestDB(t)
	seedNotificationFixture(t, db)
	h := NewNotificationHandler(service.NewNotificationService(db))

	unauthorizedRecorder := httptest.NewRecorder()
	unauthorizedContext, _ := gin.CreateTestContext(unauthorizedRecorder)
	unauthorizedContext.Request = httptest.NewRequest(http.MethodGet, "/notifications", nil)
	h.List(unauthorizedContext)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without user id, got %d", unauthorizedRecorder.Code)
	}

	invalidRecorder := httptest.NewRecorder()
	invalidContext, _ := gin.CreateTestContext(invalidRecorder)
	invalidContext.Request = httptest.NewRequest(http.MethodGet, "/notifications?limit=0", nil)
	invalidContext.Set("user_id", int64(601))
	h.List(invalidContext)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid limit, got %d", invalidRecorder.Code)
	}
}

func TestNotificationHandlerMarkReadUpdatesNotification(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupNotificationTestDB(t)
	seedNotificationFixture(t, db)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "8001"}}
	c.Request = httptest.NewRequest(http.MethodPatch, "/notifications/8001/read", nil)
	c.Set("user_id", int64(601))

	h := NewNotificationHandler(service.NewNotificationService(db))
	h.MarkRead(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("not implemented")) {
		t.Fatalf("expected mark-read payload, got %s", recorder.Body.String())
	}

	var notification model.Notification
	if err := db.First(&notification, 8001).Error; err != nil {
		t.Fatalf("failed to reload notification: %v", err)
	}
	if !notification.IsRead {
		t.Fatalf("expected notification to be marked read")
	}
}
