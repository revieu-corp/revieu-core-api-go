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
	if notification.ReadAt == nil {
		t.Fatal("expected read_at to be populated")
	}
	firstReadAt := *notification.ReadAt

	secondRecorder := httptest.NewRecorder()
	secondContext, _ := gin.CreateTestContext(secondRecorder)
	secondContext.Params = gin.Params{{Key: "id", Value: "8001"}}
	secondContext.Request = httptest.NewRequest(http.MethodPatch, "/notifications/8001/read", nil)
	secondContext.Set("user_id", int64(601))
	h.MarkRead(secondContext)
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("expected repeated mark-read status 200, got %d", secondRecorder.Code)
	}
	if err := db.First(&notification, 8001).Error; err != nil {
		t.Fatalf("failed to reload notification after repeated mark-read: %v", err)
	}
	if notification.ReadAt == nil || !notification.ReadAt.Equal(firstReadAt) {
		t.Fatalf("expected repeated mark-read to preserve read_at, got %v", notification.ReadAt)
	}
}

func TestNotificationHandlerMarkReadEnforcesOwnershipAndInvalidIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupNotificationTestDB(t)
	seedNotificationFixture(t, db)
	if err := db.Create(&model.User{ID: 602, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create second user: %v", err)
	}
	if err := db.Create(&model.Notification{ID: 8002, UserID: 602, Type: "private", Title: "Private"}).Error; err != nil {
		t.Fatalf("failed to create second notification: %v", err)
	}
	h := NewNotificationHandler(service.NewNotificationService(db))

	mark := func(userID int64, id string, authenticated bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "id", Value: id}}
		c.Request = httptest.NewRequest(http.MethodPatch, "/notifications/"+id+"/read", nil)
		if authenticated {
			c.Set("user_id", userID)
		}
		h.MarkRead(c)
		return recorder
	}

	for _, testCase := range []struct {
		name          string
		userID        int64
		id            string
		authenticated bool
		expected      int
	}{
		{name: "other-owner", userID: 601, id: "8002", authenticated: true, expected: http.StatusForbidden},
		{name: "missing", userID: 601, id: "9999", authenticated: true, expected: http.StatusNotFound},
		{name: "invalid-id", userID: 601, id: "0", authenticated: true, expected: http.StatusBadRequest},
		{name: "unauthenticated", userID: 0, id: "8001", authenticated: false, expected: http.StatusUnauthorized},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := mark(testCase.userID, testCase.id, testCase.authenticated)
			if recorder.Code != testCase.expected {
				t.Fatalf("expected status %d, got %d: %s", testCase.expected, recorder.Code, recorder.Body.String())
			}
		})
	}
}
