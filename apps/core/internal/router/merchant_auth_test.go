package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
)

func TestVerifiedMerchantGateBlocksUnverifiedPublishAndAllowsApprovedMerchant(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB

	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	merchant := model.Merchant{
		Name:               "Pending Merchant",
		UserID:             &ownerAuth.UserID,
		VerificationStatus: "pending",
	}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create pending merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Draft Store", Status: 0}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create draft store: %v", err)
	}

	w := merchantRequest(r, http.MethodPost, "/api/v1/merchant/stores/"+int64String(store.ID)+"/activate", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected pending merchant activation 403, got %d (body=%s)", w.Code, w.Body.String())
	}
	var errorResponse map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &errorResponse); err != nil {
		t.Fatalf("failed to decode gate response: %v", err)
	}
	if errorResponse["error"] != "merchant verification required" {
		t.Fatalf("unexpected pending merchant error: %q", errorResponse["error"])
	}

	var draft model.Store
	if err := db.First(&draft, store.ID).Error; err != nil {
		t.Fatalf("failed to reload draft store: %v", err)
	}
	if draft.Status != 0 {
		t.Fatalf("blocked activation changed draft store status to %d", draft.Status)
	}

	if err := db.Model(&merchant).Update("verification_status", "rejected").Error; err != nil {
		t.Fatalf("failed to set rejected merchant fixture: %v", err)
	}
	w = merchantRequest(r, http.MethodPost, "/api/v1/merchant/stores/"+int64String(store.ID)+"/activate", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected rejected merchant activation 403, got %d (body=%s)", w.Code, w.Body.String())
	}

	if err := db.Model(&merchant).Update("verification_status", "verified").Error; err != nil {
		t.Fatalf("failed to approve merchant fixture: %v", err)
	}
	w = merchantRequest(r, http.MethodPost, "/api/v1/merchant/stores/"+int64String(store.ID)+"/activate", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected verified merchant activation 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if err := db.First(&draft, store.ID).Error; err != nil {
		t.Fatalf("failed to reload published store: %v", err)
	}
	if draft.Status != 1 {
		t.Fatalf("expected verified merchant activation to publish store, got status %d", draft.Status)
	}
}

func TestMerchantGateBlocksPublishActionsWithoutMerchantAccount(t *testing.T) {
	r, tok := setupAPITest(t)

	w := merchantRequest(r, http.MethodPost, "/api/v1/merchant/stores/1/activate", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected user without merchant account 403, got %d (body=%s)", w.Code, w.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode merchant-account gate response: %v", err)
	}
	if response["error"] != "merchant account required" {
		t.Fatalf("unexpected merchant-account error: %q", response["error"])
	}
}

func merchantRequest(r http.Handler, method, path, tok string, body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(string(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	return w
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
}
