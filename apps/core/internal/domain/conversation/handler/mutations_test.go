package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/conversation/service"
)

func TestConversationMutationHandlersPersistActions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupConversationTestDB(t)
	seedConversationFixture(t, db)
	h := NewConversationHandler(service.NewConversationService(db))

	clearRecorder := httptest.NewRecorder()
	clearCtx, _ := gin.CreateTestContext(clearRecorder)
	clearCtx.Params = gin.Params{{Key: "id", Value: "9001"}}
	clearCtx.Set("user_id", int64(501))
	clearCtx.Request = httptest.NewRequest(http.MethodDelete, "/conversations/9001/messages", nil)
	h.ClearMessages(clearCtx)
	if clearRecorder.Code != http.StatusOK {
		t.Fatalf("expected clear 200, got %d: %s", clearRecorder.Code, clearRecorder.Body.String())
	}

	deleteRecorder := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRecorder)
	deleteCtx.Params = gin.Params{{Key: "id", Value: "9001"}}
	deleteCtx.Set("user_id", int64(501))
	deleteCtx.Request = httptest.NewRequest(http.MethodDelete, "/conversations/9001", nil)
	h.Delete(deleteCtx)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
}
