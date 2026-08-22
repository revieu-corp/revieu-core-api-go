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

func TestConversationHandlerCreatePreservesParticipantsAndRejectsInvalidLists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupConversationTestDB(t)
	seedConversationFixture(t, db)
	if err := db.Create(&model.User{ID: 503, Role: "user", Status: 1}).Error; err != nil {
		t.Fatalf("failed to create disabled user: %v", err)
	}
	h := NewConversationHandler(service.NewConversationService(db))

	create := func(payload string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/conversations", bytes.NewBufferString(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", int64(501))
		h.Create(c)
		return recorder
	}

	created := create(`{"title":"Support chat","type":"group","participant_ids":[502]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", created.Code, created.Body.String())
	}
	var createdResponse struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdResponse); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	var participants []model.ConversationParticipant
	if err := db.Where("conversation_id = ?", createdResponse.Data.ID).Order("user_id asc").Find(&participants).Error; err != nil {
		t.Fatalf("failed to load created participants: %v", err)
	}
	if len(participants) != 2 || participants[0].UserID != 501 || participants[1].UserID != 502 {
		t.Fatalf("expected current user and selected participant, got %+v", participants)
	}

	invalidPayloads := []string{
		`{"title":"No participants","type":"group","participant_ids":[]}`,
		`{"title":"Duplicate participant","type":"group","participant_ids":[502,502]}`,
		`{"title":"Unknown participant","type":"group","participant_ids":[999]}`,
		`{"title":"Disabled participant","type":"group","participant_ids":[503]}`,
	}
	for _, payload := range invalidPayloads {
		recorder := create(payload)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected invalid payload to return 400, got %d: %s", recorder.Code, recorder.Body.String())
		}
	}

	var conversationCount int64
	if err := db.Model(&model.Conversation{}).Count(&conversationCount).Error; err != nil {
		t.Fatalf("failed to count conversations: %v", err)
	}
	if conversationCount != 2 {
		t.Fatalf("invalid requests created extra conversations; expected 2, got %d", conversationCount)
	}
}

func TestConversationHandlerCreateReusesExistingDirectConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupConversationTestDB(t)
	seedConversationFixture(t, db)
	h := NewConversationHandler(service.NewConversationService(db))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/conversations", bytes.NewBufferString(`{"title":"New title","type":"direct","participant_ids":[502]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", int64(501))
	h.Create(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for existing direct conversation, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode existing conversation response: %v", err)
	}
	if response.Data.ID != 9001 {
		t.Fatalf("expected existing conversation 9001, got %d", response.Data.ID)
	}
	var conversationCount int64
	if err := db.Model(&model.Conversation{}).Count(&conversationCount).Error; err != nil {
		t.Fatalf("failed to count conversations: %v", err)
	}
	if conversationCount != 1 {
		t.Fatalf("expected no duplicate direct conversation, got %d conversations", conversationCount)
	}
}

func TestConversationHandlerCreateRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupConversationTestDB(t)
	h := NewConversationHandler(service.NewConversationService(db))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/conversations", bytes.NewBufferString(`{"title":"Support chat","participant_ids":[502]}`))
	h.Create(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without user id, got %d", recorder.Code)
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
