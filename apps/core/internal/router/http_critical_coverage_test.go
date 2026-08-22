package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
)

func TestCriticalProtectedRoutesRequireJWT(t *testing.T) {
	r, _ := setupAPITest(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{name: "conversations list", method: http.MethodGet, path: "/api/v1/conversations"},
		{name: "conversations create", method: http.MethodPost, path: "/api/v1/conversations"},
		{name: "conversation messages", method: http.MethodGet, path: "/api/v1/conversations/1/messages"},
		{name: "conversation send", method: http.MethodPost, path: "/api/v1/conversations/1/messages"},
		{name: "conversation settings", method: http.MethodPatch, path: "/api/v1/conversations/1/settings"},
		{name: "notifications list", method: http.MethodGet, path: "/api/v1/notifications"},
		{name: "notifications read all", method: http.MethodPost, path: "/api/v1/notifications/read-all"},
		{name: "orders list", method: http.MethodGet, path: "/api/v1/orders"},
		{name: "orders create", method: http.MethodPost, path: "/api/v1/orders"},
		{name: "order pay", method: http.MethodPost, path: "/api/v1/orders/1/pay"},
		{name: "payments create", method: http.MethodPost, path: "/api/v1/payments"},
		{name: "payment detail", method: http.MethodGet, path: "/api/v1/payments/1"},
		{name: "media upload", method: http.MethodPost, path: "/api/v1/media/uploads"},
		{name: "media presigned urls", method: http.MethodPost, path: "/api/v1/media/presigned-urls"},
		{name: "media analysis", method: http.MethodPost, path: "/api/v1/media/1/analysis"},
		{name: "AI suggestions", method: http.MethodPost, path: "/api/v1/ai/reviews/suggestions"},
		{name: "merchant verification status", method: http.MethodGet, path: "/api/v1/merchant/verification"},
		{name: "merchant verification submit", method: http.MethodPost, path: "/api/v1/merchant/verification"},
		{name: "voucher list", method: http.MethodGet, path: "/api/v1/vouchers"},
		{name: "voucher create", method: http.MethodPost, path: "/api/v1/vouchers"},
		{name: "voucher use", method: http.MethodPatch, path: "/api/v1/vouchers/1/use"},
		{name: "voucher status", method: http.MethodPatch, path: "/api/v1/vouchers/1/status"},
		{name: "merchant voucher scan", method: http.MethodGet, path: "/api/v1/merchant/vouchers/scan"},
		{name: "merchant voucher redeem", method: http.MethodPost, path: "/api/v1/merchant/vouchers/redeem-by-token"},
		{name: "merchant dishes list", method: http.MethodGet, path: "/api/v1/merchant/dishes"},
		{name: "merchant dish create", method: http.MethodPost, path: "/api/v1/merchant/dishes"},
		{name: "admin reports", method: http.MethodGet, path: "/api/v1/admin/reports"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			r.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 before handler execution, got %d (body=%s)", w.Code, w.Body.String())
			}

			var response map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("expected JSON error response: %v", err)
			}
			if strings.TrimSpace(response["error"]) == "" {
				t.Fatalf("expected a stable error field, got %s", w.Body.String())
			}
		})
	}
}

func TestConversationHTTPEnforcesParticipantAccessAndPersistsMutations(t *testing.T) {
	r, ownerToken := setupAPITest(t)
	db := database.DB

	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}

	member := model.User{Role: "user", Status: 0}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("failed to create member: %v", err)
	}

	outsider := model.User{Role: "user", Status: 0}
	if err := db.Create(&outsider).Error; err != nil {
		t.Fatalf("failed to create outsider: %v", err)
	}
	outsiderToken := issueAPITestToken(t, outsider, "outsider@example.com")

	conversation := model.Conversation{
		Type:      "direct",
		Title:     "Merchant partnership",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	participants := []model.ConversationParticipant{
		{ConversationID: conversation.ID, UserID: ownerAuth.UserID, Role: "owner", JoinedAt: time.Now().UTC()},
		{ConversationID: conversation.ID, UserID: member.ID, Role: "member", JoinedAt: time.Now().UTC()},
	}
	if err := db.Create(&participants).Error; err != nil {
		t.Fatalf("failed to create participants: %v", err)
	}

	getPath := fmt.Sprintf("/api/v1/conversations/%d/messages", conversation.ID)
	w := requestWithToken(r, http.MethodGet, fmt.Sprintf("/api/v1/conversations/%d", conversation.ID), ownerToken, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected unknown conversation subroute to remain 404, got %d", w.Code)
	}

	w = requestWithToken(r, http.MethodGet, "/api/v1/conversations", ownerToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected owner conversation list 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var listResponse struct {
		Data []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResponse); err != nil {
		t.Fatalf("failed to decode conversation list: %v", err)
	}
	if len(listResponse.Data) != 1 || listResponse.Data[0].ID != conversation.ID || listResponse.Data[0].Title != conversation.Title {
		t.Fatalf("unexpected conversation list: %+v", listResponse.Data)
	}

	w = requestWithToken(r, http.MethodGet, getPath, ownerToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected participant messages 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var emptyMessages struct {
		Data []model.Message `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &emptyMessages); err != nil {
		t.Fatalf("failed to decode participant messages: %v", err)
	}
	if len(emptyMessages.Data) != 0 {
		t.Fatalf("expected empty initial message list, got %d", len(emptyMessages.Data))
	}

	w = requestWithToken(r, http.MethodPost, getPath, ownerToken, []byte(`{"content":"Please confirm the launch date."}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected participant send 201, got %d (body=%s)", w.Code, w.Body.String())
	}
	var sendResponse struct {
		Data struct {
			ConversationID int64  `json:"conversation_id"`
			Content        string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &sendResponse); err != nil {
		t.Fatalf("failed to decode sent message: %v", err)
	}
	if sendResponse.Data.ConversationID != conversation.ID || sendResponse.Data.Content != "Please confirm the launch date." {
		t.Fatalf("unexpected sent message payload: %+v", sendResponse.Data)
	}

	w = requestWithToken(r, http.MethodPatch, fmt.Sprintf("/api/v1/conversations/%d/settings", conversation.ID), ownerToken, []byte(`{"is_muted":true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected participant settings 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var ownerMembership model.ConversationParticipant
	if err := db.Where("conversation_id = ? AND user_id = ?", conversation.ID, ownerAuth.UserID).First(&ownerMembership).Error; err != nil {
		t.Fatalf("failed to reload owner membership: %v", err)
	}
	if !ownerMembership.IsMuted {
		t.Fatal("expected participant settings mutation to persist")
	}

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{name: "messages", method: http.MethodGet, path: getPath},
		{name: "send message", method: http.MethodPost, path: getPath, body: []byte(`{"content":"unauthorized access"}`)},
		{name: "settings", method: http.MethodPatch, path: fmt.Sprintf("/api/v1/conversations/%d/settings", conversation.ID), body: []byte(`{"is_muted":false}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := requestWithToken(r, tc.method, tc.path, outsiderToken, tc.body)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected non-participant 403, got %d (body=%s)", w.Code, w.Body.String())
			}
		})
	}

	var messageCount int64
	if err := db.Model(&model.Message{}).Where("conversation_id = ?", conversation.ID).Count(&messageCount).Error; err != nil {
		t.Fatalf("failed to count conversation messages: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("non-participant request changed message count: got %d", messageCount)
	}
	if err := db.Where("conversation_id = ? AND user_id = ?", conversation.ID, ownerAuth.UserID).First(&ownerMembership).Error; err != nil {
		t.Fatalf("failed to reload owner membership: %v", err)
	}
	if !ownerMembership.IsMuted {
		t.Fatal("non-participant settings request changed owner membership")
	}
}

func TestConversationHTTPRejectsMalformedID(t *testing.T) {
	r, tok := setupAPITest(t)

	w := requestWithToken(r, http.MethodGet, "/api/v1/conversations/not-an-id/messages", tok, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed conversation id 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestMerchantVerificationHTTPValidatesAndPersistsStatus(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB

	w := requestWithToken(r, http.MethodPost, "/api/v1/merchant/verification", tok, []byte(`{"document_type":"business_license"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected incomplete verification 400, got %d (body=%s)", w.Code, w.Body.String())
	}

	w = requestWithToken(r, http.MethodPost, "/api/v1/merchant/verification", tok, []byte(`{"document_type":"business_license","document_url":"https://example.com/license.pdf","business_license":"LIC-HTTP-001"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected verification submit 201, got %d (body=%s)", w.Code, w.Body.String())
	}
	var submitResponse struct {
		Data struct {
			Status          string `json:"status"`
			DocumentURL     string `json:"document_url"`
			BusinessLicense string `json:"business_license"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &submitResponse); err != nil {
		t.Fatalf("failed to decode verification response: %v", err)
	}
	if submitResponse.Data.Status != "pending" || submitResponse.Data.DocumentURL != "https://example.com/license.pdf" || submitResponse.Data.BusinessLicense != "LIC-HTTP-001" {
		t.Fatalf("unexpected verification response: %+v", submitResponse.Data)
	}

	w = requestWithToken(r, http.MethodGet, "/api/v1/merchant/verification", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected verification status 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var statusResponse struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &statusResponse); err != nil {
		t.Fatalf("failed to decode verification status: %v", err)
	}
	if statusResponse.Data.Status != "pending" {
		t.Fatalf("expected pending verification status, got %q", statusResponse.Data.Status)
	}

	var merchant model.Merchant
	if err := db.Where("name = ?", "merchant").First(&merchant).Error; err != nil {
		t.Fatalf("failed to load auto-created merchant: %v", err)
	}
	if merchant.VerificationStatus != "pending" {
		t.Fatalf("expected merchant verification_status=pending, got %q", merchant.VerificationStatus)
	}
	var verification model.MerchantVerification
	if err := db.Where("merchant_id = ?", merchant.ID).First(&verification).Error; err != nil {
		t.Fatalf("failed to load persisted verification: %v", err)
	}
	if verification.BusinessLicense != "LIC-HTTP-001" {
		t.Fatalf("expected persisted business license, got %q", verification.BusinessLicense)
	}
}

func TestOAuthCallbackRejectsMissingAuthorizationCode(t *testing.T) {
	r, _ := setupAPITest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback/google?state=https%3A%2F%2Fmerchant.revieu.test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected missing OAuth code 400, got %d (body=%s)", w.Code, w.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode OAuth error: %v", err)
	}
	if response["error"] != "missing authorization code" {
		t.Fatalf("unexpected OAuth error: %q", response["error"])
	}
}

func TestGoogleLoginRejectsMissingOAuthConfiguration(t *testing.T) {
	r, _ := setupAPITest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login/google", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected unconfigured Google OAuth 500, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func requestWithToken(r http.Handler, method, path, token string, body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	return w
}
