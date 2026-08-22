package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/conversation/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupConversationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.UserProfile{},
		&model.Conversation{},
		&model.ConversationParticipant{},
		&model.Message{},
	); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	return db
}

func seedConversationFixture(t *testing.T, db *gorm.DB) {
	t.Helper()

	merchantUser := model.User{ID: 501, Role: "merchant", Status: 0}
	customerUser := model.User{ID: 502, Role: "user", Status: 0}
	for _, user := range []model.User{merchantUser, customerUser} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", user.ID, err)
		}
	}

	profiles := []model.UserProfile{
		{UserID: merchantUser.ID, Nickname: "Merchant Jane", AvatarURL: "https://example.com/merchant.png"},
		{UserID: customerUser.ID, Nickname: "Sarah Johnson", AvatarURL: "https://example.com/customer.png"},
	}
	if err := db.Create(&profiles).Error; err != nil {
		t.Fatalf("failed to create profiles: %v", err)
	}

	conversation := model.Conversation{
		ID:        9001,
		Type:      "direct",
		Title:     "Sarah Johnson",
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Minute),
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	participants := []model.ConversationParticipant{
		{ConversationID: conversation.ID, UserID: merchantUser.ID, Role: "owner", JoinedAt: time.Now().Add(-time.Hour)},
		{ConversationID: conversation.ID, UserID: customerUser.ID, Role: "member", JoinedAt: time.Now().Add(-time.Hour)},
	}
	if err := db.Create(&participants).Error; err != nil {
		t.Fatalf("failed to create participants: %v", err)
	}

	message := model.Message{
		ID:             7001,
		ConversationID: conversation.ID,
		SenderID:       customerUser.ID,
		Content:        "Question about cake availability",
		MessageType:    "text",
		IsRead:         false,
		CreatedAt:      time.Now().Add(-5 * time.Minute),
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("failed to create message: %v", err)
	}
}

func TestConversationHandlerListReturnsConversationSummaries(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupConversationTestDB(t)
	seedConversationFixture(t, db)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/conversations", nil)
	c.Set("user_id", int64(501))

	h := NewConversationHandler(service.NewConversationService(db))
	h.List(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("not implemented")) {
		t.Fatalf("expected conversation list payload, got %s", recorder.Body.String())
	}

	var response struct {
		Data []struct {
			ID          int64  `json:"id"`
			Title       string `json:"title"`
			LastMessage string `json:"last_message"`
			UnreadCount int    `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(response.Data) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(response.Data))
	}
	if response.Data[0].Title != "Sarah Johnson" {
		t.Fatalf("expected title Sarah Johnson, got %q", response.Data[0].Title)
	}
	if response.Data[0].LastMessage != "Question about cake availability" {
		t.Fatalf("expected last message to be returned, got %q", response.Data[0].LastMessage)
	}
	if response.Data[0].UnreadCount != 1 {
		t.Fatalf("expected unread_count=1, got %d", response.Data[0].UnreadCount)
	}
}

func TestConversationHandlerMessagesSupportsCursorPaginationAndMarksRead(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupConversationTestDB(t)
	seedConversationFixture(t, db)
	for _, message := range []model.Message{
		{ID: 7002, ConversationID: 9001, SenderID: 502, Content: "Second message", MessageType: "text", CreatedAt: time.Now().Add(-2 * time.Minute)},
		{ID: 7003, ConversationID: 9001, SenderID: 502, Content: "Latest message", MessageType: "text", CreatedAt: time.Now().Add(-time.Minute)},
	} {
		if err := db.Create(&message).Error; err != nil {
			t.Fatalf("failed to create message %d: %v", message.ID, err)
		}
	}
	h := NewConversationHandler(service.NewConversationService(db))

	requestPage := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "id", Value: "9001"}}
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		c.Set("user_id", int64(501))
		h.Messages(c)
		return recorder
	}

	firstRecorder := requestPage("/conversations/9001/messages?limit=2")
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
		t.Fatalf("failed to decode first page: %v", err)
	}
	if len(firstPage.Data) != 2 || firstPage.Data[0].ID != 7002 || firstPage.Data[1].ID != 7003 {
		t.Fatalf("expected newest page in ascending order [7002 7003], got %+v", firstPage.Data)
	}
	if firstPage.Cursor == nil || *firstPage.Cursor != 7002 {
		t.Fatalf("expected cursor 7002, got %v", firstPage.Cursor)
	}

	secondRecorder := requestPage("/conversations/9001/messages?limit=2&cursor=7002")
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
		t.Fatalf("failed to decode second page: %v", err)
	}
	if len(secondPage.Data) != 1 || secondPage.Data[0].ID != 7001 || secondPage.Cursor != nil {
		t.Fatalf("expected oldest page [7001] without cursor, got %+v cursor=%v", secondPage.Data, secondPage.Cursor)
	}

	var membership model.ConversationParticipant
	if err := db.Where("conversation_id = ? AND user_id = ?", 9001, 501).First(&membership).Error; err != nil {
		t.Fatalf("failed to load membership: %v", err)
	}
	if membership.LastReadAt.IsZero() {
		t.Fatal("expected opening messages to update last_read_at")
	}
	var unread int64
	if err := db.Model(&model.Message{}).Where("conversation_id = ? AND sender_id = ? AND is_read = ?", 9001, 502, false).Count(&unread).Error; err != nil {
		t.Fatalf("failed to count unread messages: %v", err)
	}
	if unread != 0 {
		t.Fatalf("expected all incoming messages to be marked read, got %d unread", unread)
	}
}

func TestConversationHandlerMessagesDistinguishesForbiddenAndMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupConversationTestDB(t)
	seedConversationFixture(t, db)
	if err := db.Create(&model.User{ID: 503, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create unrelated user: %v", err)
	}
	h := NewConversationHandler(service.NewConversationService(db))

	for _, testCase := range []struct {
		name           string
		userID         int64
		conversationID string
		expected       int
	}{
		{name: "non-member", userID: 503, conversationID: "9001", expected: http.StatusForbidden},
		{name: "missing", userID: 501, conversationID: "9999", expected: http.StatusNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Params = gin.Params{{Key: "id", Value: testCase.conversationID}}
			c.Request = httptest.NewRequest(http.MethodGet, "/conversations/"+testCase.conversationID+"/messages", nil)
			c.Set("user_id", testCase.userID)
			h.Messages(c)
			if recorder.Code != testCase.expected {
				t.Fatalf("expected status %d, got %d: %s", testCase.expected, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestConversationHandlerMessagesRejectsUnauthenticatedAndInvalidPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupConversationTestDB(t)
	seedConversationFixture(t, db)
	h := NewConversationHandler(service.NewConversationService(db))

	unauthorizedRecorder := httptest.NewRecorder()
	unauthorizedContext, _ := gin.CreateTestContext(unauthorizedRecorder)
	unauthorizedContext.Params = gin.Params{{Key: "id", Value: "9001"}}
	unauthorizedContext.Request = httptest.NewRequest(http.MethodGet, "/conversations/9001/messages", nil)
	h.Messages(unauthorizedContext)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without user id, got %d", unauthorizedRecorder.Code)
	}

	invalidRecorder := httptest.NewRecorder()
	invalidContext, _ := gin.CreateTestContext(invalidRecorder)
	invalidContext.Params = gin.Params{{Key: "id", Value: "9001"}}
	invalidContext.Request = httptest.NewRequest(http.MethodGet, "/conversations/9001/messages?limit=0", nil)
	invalidContext.Set("user_id", int64(501))
	h.Messages(invalidContext)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid limit, got %d", invalidRecorder.Code)
	}
}

func TestConversationHandlerSendMessagePersistsMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupConversationTestDB(t)
	seedConversationFixture(t, db)

	body := []byte(`{"content":"It will be ready at 5 PM."}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "9001"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/conversations/9001/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", int64(501))

	h := NewConversationHandler(service.NewConversationService(db))
	h.SendMessage(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", recorder.Code)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("not implemented")) {
		t.Fatalf("expected send message payload, got %s", recorder.Body.String())
	}

	var count int64
	if err := db.Model(&model.Message{}).Where("conversation_id = ?", 9001).Count(&count).Error; err != nil {
		t.Fatalf("failed to count messages: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 messages after send, got %d", count)
	}
}
