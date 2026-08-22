package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/token"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
)

func setupAPITest(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := testutil.SetupTestDB(t)
	database.DB = db

	cfg := &config.Config{
		Server:      config.ServerConfig{APIBasePath: "/api/v1", Mode: "debug"},
		JWT:         config.JWTConfig{Secret: "test-secret", ExpireHour: 24},
		FrontendURL: "https://merchant.revieu.test",
	}

	r := gin.New()
	Setup(r, cfg)

	user := model.User{Role: "user", Status: 0}
	_ = db.Create(&user).Error
	auth := model.UserAuth{UserID: user.ID, IdentityType: "email", Identifier: "user@example.com"}
	_ = db.Create(&auth).Error
	tok, _ := token.New(cfg.JWT).GenerateToken(&user, &auth)

	return r, tok
}

func issueAPITestToken(t *testing.T, user model.User, identifier string) string {
	t.Helper()

	db := database.DB
	auth := model.UserAuth{UserID: user.ID, IdentityType: "email", Identifier: identifier}
	if err := db.Create(&auth).Error; err != nil {
		t.Fatalf("failed to create auth: %v", err)
	}
	tok, err := token.New(config.JWTConfig{Secret: "test-secret", ExpireHour: 24}).GenerateToken(&user, &auth)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	return tok
}

func TestFeedHome(t *testing.T) {
	r, _ := setupAPITest(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/home", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMerchantsList(t *testing.T) {
	r, _ := setupAPITest(t)

	db := database.DB
	_ = db.Create(&model.Merchant{Name: "Cafe", Category: "food", VerificationStatus: "verified", Status: 0}).Error

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/merchants", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMerchantDetail(t *testing.T) {
	r, _ := setupAPITest(t)

	db := database.DB
	m := model.Merchant{Name: "Shop", VerificationStatus: "verified", Status: 0}
	_ = db.Create(&m).Error

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/merchants/%d", m.ID), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMerchantReviews(t *testing.T) {
	r, _ := setupAPITest(t)

	db := database.DB
	user := model.User{Role: "user", Status: 0}
	_ = db.Create(&user).Error
	m := model.Merchant{Name: "Shop", VerificationStatus: "verified", Status: 0}
	_ = db.Create(&m).Error
	_ = db.Create(&model.Review{UserID: user.ID, MerchantID: m.ID, Rating: 4, Content: "nice"}).Error

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/merchants/%d/reviews", m.ID), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMerchantsListOnlyShowsPublicMerchants(t *testing.T) {
	r, _ := setupAPITest(t)

	db := database.DB
	if err := db.Create(&model.Merchant{Name: "Public", VerificationStatus: "verified", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create public merchant: %v", err)
	}
	if err := db.Create(&model.Merchant{Name: "Unverified", VerificationStatus: "unverified", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create unverified merchant: %v", err)
	}
	if err := db.Create(&model.Merchant{Name: "Inactive", VerificationStatus: "verified", Status: 1}).Error; err != nil {
		t.Fatalf("failed to create inactive merchant: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/merchants", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode merchant list response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 public merchant, got %d", len(resp.Data))
	}
	name, _ := resp.Data[0]["name"].(string)
	if name != "Public" {
		t.Fatalf("unexpected merchant returned: %s", name)
	}
}

func TestMerchantDetailReturnsNotFoundWhenNotPublic(t *testing.T) {
	r, _ := setupAPITest(t)

	db := database.DB
	publicMerchant := model.Merchant{Name: "Public", VerificationStatus: "verified", Status: 0}
	if err := db.Create(&publicMerchant).Error; err != nil {
		t.Fatalf("failed to create public merchant: %v", err)
	}
	hiddenMerchant := model.Merchant{Name: "Hidden", VerificationStatus: "unverified", Status: 0}
	if err := db.Create(&hiddenMerchant).Error; err != nil {
		t.Fatalf("failed to create hidden merchant: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/merchants/%d", publicMerchant.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected public merchant detail 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/merchants/%d", hiddenMerchant.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected hidden merchant detail 404, got %d", w.Code)
	}
}

func TestMerchantReviewsReturnsNotFoundWhenMerchantMissingOrNotPublic(t *testing.T) {
	r, _ := setupAPITest(t)

	db := database.DB
	hidden := model.Merchant{Name: "Hidden", VerificationStatus: "unverified", Status: 0}
	if err := db.Create(&hidden).Error; err != nil {
		t.Fatalf("failed to create hidden merchant: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/merchants/99999/reviews", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected missing merchant reviews 404, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/merchants/%d/reviews", hidden.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected hidden merchant reviews 404, got %d", w.Code)
	}
}

func TestStoreCreateActivateAndPublicVisibility(t *testing.T) {
	r, tok := setupAPITest(t)

	createBody := strings.NewReader(`{"name":"Draft Store","address":"Austin"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/merchant/stores", createBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d", w.Code)
	}

	var created struct {
		Data model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.Data.Status != 0 {
		t.Fatalf("expected created store status 0(draft), got %d", created.Data.Status)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/stores", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d", w.Code)
	}
	var listed struct {
		Data []model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("failed to decode stores list response: %v", err)
	}
	if len(listed.Data) != 0 {
		t.Fatalf("expected no public stores before activation, got %d", len(listed.Data))
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/merchant/stores/%d/activate", created.Data.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected activate 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/stores", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected list 200 after activate, got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("failed to decode stores list response after activate: %v", err)
	}
	if len(listed.Data) != 1 {
		t.Fatalf("expected one public store after activation, got %d", len(listed.Data))
	}
	if listed.Data[0].ID != created.Data.ID {
		t.Fatalf("unexpected public store id: got %d, want %d", listed.Data[0].ID, created.Data.ID)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/merchant/stores/%d/deactivate", created.Data.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected deactivate 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stores/%d", created.Data.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected detail 404 after deactivate, got %d", w.Code)
	}
}

func TestMerchantStoresListHonorsLimitQuery(t *testing.T) {
	r, tok := setupAPITest(t)

	createStore := func(name string) model.Store {
		t.Helper()

		body := strings.NewReader(fmt.Sprintf(`{"name":"%s","address":"Austin"}`, name))
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/merchant/stores", body)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected create 201, got %d", w.Code)
		}

		var created struct {
			Data model.Store `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatalf("failed to decode create response: %v", err)
		}
		return created.Data
	}

	first := createStore("Store One")
	second := createStore("Store Two")
	third := createStore("Store Three")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/merchant/stores?limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d", w.Code)
	}

	var listed struct {
		Data []model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("failed to decode merchant stores list response: %v", err)
	}
	if len(listed.Data) != 2 {
		t.Fatalf("expected 2 merchant stores when limit=2, got %d", len(listed.Data))
	}
	if listed.Data[0].ID != third.ID || listed.Data[1].ID != second.ID {
		t.Fatalf("expected newest two stores [%d %d], got [%d %d]", third.ID, second.ID, listed.Data[0].ID, listed.Data[1].ID)
	}
	for _, store := range listed.Data {
		if store.ID == first.ID {
			t.Fatalf("did not expect oldest store %d when limit=2", first.ID)
		}
	}
}

func TestStoreCreateWithCategoriesReflectedInCategoryFilter(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB

	category := model.Category{Name: "Cafe"}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	createBody := strings.NewReader(fmt.Sprintf(`{"name":"Categorized Store","address":"Austin","category_ids":[%d]}`, category.ID))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/merchant/stores", createBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d", w.Code)
	}

	var created struct {
		Data model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/merchant/stores/%d/activate", created.Data.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected activate 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stores?category=%d", category.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected category list 200, got %d", w.Code)
	}

	var listed struct {
		Data []model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("failed to decode category list response: %v", err)
	}
	found := false
	for _, s := range listed.Data {
		if s.ID == created.Data.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected created categorized store to appear in category filter result")
	}
}

func TestStoreCreateReturnsBadRequestWhenCategoryInvalid(t *testing.T) {
	r, tok := setupAPITest(t)

	createBody := strings.NewReader(`{"name":"Broken Category Store","category_ids":[99999]}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/merchant/stores", createBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected create 400 for invalid categories, got %d", w.Code)
	}
}

func TestStoreUpdateReturnsBadRequestWhenCategoryInvalid(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB

	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	ownerMerchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerAuth.UserID}
	if err := db.Create(&ownerMerchant).Error; err != nil {
		t.Fatalf("failed to create owner merchant: %v", err)
	}
	store := model.Store{MerchantID: ownerMerchant.ID, Name: "Category Update Store"}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	body := strings.NewReader(`{"category_ids":[99999]}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/merchant/stores/%d", store.ID), body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected update 400 for invalid categories, got %d", w.Code)
	}
}

func TestStoreActivateForbiddenForNonOwner(t *testing.T) {
	r, ownerToken := setupAPITest(t)

	createBody := strings.NewReader(`{"name":"Owner Store","address":"Austin"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/merchant/stores", createBody)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d", w.Code)
	}
	var created struct {
		Data model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	db := database.DB
	other := model.User{Role: "user", Status: 0}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("failed to create other user: %v", err)
	}
	otherToken := issueAPITestToken(t, other, "other@example.com")

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/merchant/stores/%d/activate", created.Data.ID), nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden activate for non-owner, got %d", w.Code)
	}
}

func TestStoreDetailIncludesHoursAndCategories(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	merchant := model.Merchant{Name: "Detail Merchant"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Published Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	hour := model.StoreHour{StoreID: store.ID, DayOfWeek: 1, OpenTime: "09:00", CloseTime: "18:00"}
	if err := db.Create(&hour).Error; err != nil {
		t.Fatalf("failed to create store hour: %v", err)
	}
	category := model.Category{Name: "Cafe"}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("failed to create category: %v", err)
	}
	storeCategory := model.StoreCategory{StoreID: store.ID, CategoryID: category.ID}
	if err := db.Create(&storeCategory).Error; err != nil {
		t.Fatalf("failed to create store category relation: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stores/%d", store.ID), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data.Hours) != 1 {
		t.Fatalf("expected 1 hour in detail, got %d", len(resp.Data.Hours))
	}
	if len(resp.Data.Categories) != 1 {
		t.Fatalf("expected 1 category in detail, got %d", len(resp.Data.Categories))
	}
}

func TestStoreListSupportsFilteringAndCursorPagination(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	merchant := model.Merchant{Name: "Filter Merchant"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	cafe := model.Category{Name: "Cafe"}
	bakery := model.Category{Name: "Bakery"}
	if err := db.Create(&cafe).Error; err != nil {
		t.Fatalf("failed to create cafe category: %v", err)
	}
	if err := db.Create(&bakery).Error; err != nil {
		t.Fatalf("failed to create bakery category: %v", err)
	}

	createStore := func(name string, status int16, rating float32, lat, lng float64, categoryID int64) model.Store {
		store := model.Store{
			MerchantID: merchant.ID,
			Name:       name,
			Status:     status,
			AvgRating:  rating,
			Latitude:   lat,
			Longitude:  lng,
		}
		if err := db.Create(&store).Error; err != nil {
			t.Fatalf("failed to create store %s: %v", name, err)
		}
		if err := db.Create(&model.StoreCategory{StoreID: store.ID, CategoryID: categoryID}).Error; err != nil {
			t.Fatalf("failed to create store category relation for %s: %v", name, err)
		}
		return store
	}

	matchOld := createStore("Match Old", 1, 4.1, 37.7750, -122.4190, cafe.ID)
	matchNew := createStore("Match New", 1, 4.8, 37.7752, -122.4188, cafe.ID)
	_ = createStore("Low Rating", 1, 3.0, 37.7751, -122.4189, cafe.ID)
	_ = createStore("Far Away", 1, 4.9, 39.0000, -120.0000, cafe.ID)
	_ = createStore("Other Category", 1, 4.9, 37.7753, -122.4187, bakery.ID)
	_ = createStore("Draft", 0, 4.9, 37.7751, -122.4189, cafe.ID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/stores?category=Cafe&rating=4&lat=37.7750&lng=-122.4190&radius_km=5&limit=1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected first page 200, got %d", w.Code)
	}

	var firstPage struct {
		Data   []model.Store `json:"data"`
		Cursor *int64        `json:"cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("failed to decode first-page response: %v", err)
	}
	if len(firstPage.Data) != 1 {
		t.Fatalf("expected first page size 1, got %d", len(firstPage.Data))
	}
	if firstPage.Data[0].ID != matchNew.ID {
		t.Fatalf("unexpected first-page store id: got %d, want %d", firstPage.Data[0].ID, matchNew.ID)
	}
	if firstPage.Cursor == nil {
		t.Fatalf("expected first page cursor")
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stores?category=Cafe&rating=4&lat=37.7750&lng=-122.4190&radius_km=5&limit=1&cursor=%d", *firstPage.Cursor), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected second page 200, got %d", w.Code)
	}

	var secondPage struct {
		Data   []model.Store `json:"data"`
		Cursor *int64        `json:"cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("failed to decode second-page response: %v", err)
	}
	if len(secondPage.Data) != 1 {
		t.Fatalf("expected second page size 1, got %d", len(secondPage.Data))
	}
	if secondPage.Data[0].ID != matchOld.ID {
		t.Fatalf("unexpected second-page store id: got %d, want %d", secondPage.Data[0].ID, matchOld.ID)
	}
	if secondPage.Cursor != nil {
		t.Fatalf("expected second page cursor to be nil")
	}
}

func TestStoreReviewsSupportsCursorPaginationAndUserPreload(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	merchant := model.Merchant{Name: "Review Merchant"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Published Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	makeUser := func(nickname string) model.User {
		user := model.User{Role: "user", Status: 0}
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		profile := model.UserProfile{UserID: user.ID, Nickname: nickname}
		if err := db.Create(&profile).Error; err != nil {
			t.Fatalf("failed to create profile: %v", err)
		}
		return user
	}

	u1 := makeUser("u1")
	u2 := makeUser("u2")
	u3 := makeUser("u3")

	addReview := func(userID int64, content string) model.Review {
		storeID := store.ID
		review := model.Review{
			UserID:     userID,
			MerchantID: merchant.ID,
			VenueID:    merchant.ID,
			StoreID:    &storeID,
			Rating:     4.5,
			Content:    content,
		}
		if err := db.Create(&review).Error; err != nil {
			t.Fatalf("failed to create review: %v", err)
		}
		return review
	}

	oldest := addReview(u1.ID, "first")
	_ = addReview(u2.ID, "second")
	_ = addReview(u3.ID, "third")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stores/%d/reviews?limit=2", store.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected first page 200, got %d", w.Code)
	}

	var firstPage struct {
		Data   []model.Review `json:"data"`
		Cursor *int64         `json:"cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("failed to decode first-page reviews: %v", err)
	}
	if len(firstPage.Data) != 2 {
		t.Fatalf("expected first page size 2, got %d", len(firstPage.Data))
	}
	if firstPage.Cursor == nil {
		t.Fatalf("expected review cursor on first page")
	}
	if firstPage.Data[0].User == nil || firstPage.Data[0].User.Profile == nil {
		t.Fatalf("expected user/profile preloaded on reviews")
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stores/%d/reviews?limit=2&cursor=%d", store.ID, *firstPage.Cursor), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected second page 200, got %d", w.Code)
	}

	var secondPage struct {
		Data   []model.Review `json:"data"`
		Cursor *int64         `json:"cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("failed to decode second-page reviews: %v", err)
	}
	if len(secondPage.Data) != 1 {
		t.Fatalf("expected second page size 1, got %d", len(secondPage.Data))
	}
	if secondPage.Data[0].ID != oldest.ID {
		t.Fatalf("unexpected second-page review id: got %d, want %d", secondPage.Data[0].ID, oldest.ID)
	}
	if secondPage.Data[0].User == nil || secondPage.Data[0].User.Profile == nil {
		t.Fatalf("expected user/profile preloaded on second page")
	}
	if secondPage.Cursor != nil {
		t.Fatalf("expected second page cursor to be nil")
	}
}

func TestUserReviewsListSupportsPaginationAndIncludesCommentCount(t *testing.T) {
	r, tok := setupAPITest(t)

	db := database.DB
	var authUser model.User
	if err := db.First(&authUser).Error; err != nil {
		t.Fatalf("failed to load auth user: %v", err)
	}
	merchant := model.Merchant{Name: "Cafe"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	addReview := func(content string, commentCount int) model.Review {
		review := model.Review{
			UserID:       authUser.ID,
			MerchantID:   merchant.ID,
			VenueID:      merchant.ID,
			Rating:       5,
			Content:      content,
			CommentCount: commentCount,
		}
		if err := db.Create(&review).Error; err != nil {
			t.Fatalf("failed to create review %s: %v", content, err)
		}
		return review
	}

	oldest := addReview("great", 1)
	_ = addReview("better", 2)
	latest := addReview("best", 3)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/user/reviews?limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var firstPage struct {
		Reviews []struct {
			ID           int64 `json:"id"`
			CommentCount int   `json:"comment_count"`
		} `json:"reviews"`
		Total  int    `json:"total"`
		Cursor *int64 `json:"cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("failed to decode first-page response: %v", err)
	}
	if firstPage.Total != 3 {
		t.Fatalf("expected total 3, got %d", firstPage.Total)
	}
	if len(firstPage.Reviews) != 2 {
		t.Fatalf("expected first page size 2, got %d", len(firstPage.Reviews))
	}
	if firstPage.Reviews[0].ID != latest.ID {
		t.Fatalf("unexpected first review id: got %d, want %d", firstPage.Reviews[0].ID, latest.ID)
	}
	if firstPage.Reviews[0].CommentCount != 3 {
		t.Fatalf("unexpected first review comment count: got %d, want 3", firstPage.Reviews[0].CommentCount)
	}
	if firstPage.Cursor == nil {
		t.Fatalf("expected first page cursor")
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/user/reviews?limit=2&cursor=%d", *firstPage.Cursor), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected second page 200, got %d", w.Code)
	}

	var secondPage struct {
		Reviews []struct {
			ID           int64 `json:"id"`
			CommentCount int   `json:"comment_count"`
		} `json:"reviews"`
		Total  int    `json:"total"`
		Cursor *int64 `json:"cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("failed to decode second-page response: %v", err)
	}
	if len(secondPage.Reviews) != 1 {
		t.Fatalf("expected second page size 1, got %d", len(secondPage.Reviews))
	}
	if secondPage.Total != 3 {
		t.Fatalf("expected second page total 3, got %d", secondPage.Total)
	}
	if secondPage.Reviews[0].ID != oldest.ID {
		t.Fatalf("unexpected second-page review id: got %d, want %d", secondPage.Reviews[0].ID, oldest.ID)
	}
	if secondPage.Reviews[0].CommentCount != 1 {
		t.Fatalf("unexpected second-page review comment count: got %d, want 1", secondPage.Reviews[0].CommentCount)
	}
	if secondPage.Cursor != nil {
		t.Fatalf("expected second page cursor to be nil")
	}
}

func TestLegacyReviewsListRouteRemoved(t *testing.T) {
	r, tok := setupAPITest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestReviewsCreateAndDetail(t *testing.T) {
	r, tok := setupAPITest(t)

	db := database.DB
	m := model.Merchant{Name: "Cafe"}
	_ = db.Create(&m).Error

	body := strings.NewReader(fmt.Sprintf(`{"merchantId":"%d","rating":4.5,"text":"nice"}`, m.ID))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/reviews", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestReviewsCreateReturnsNotFoundWhenMerchantMissing(t *testing.T) {
	r, tok := setupAPITest(t)

	body := strings.NewReader(`{"merchantId":"99999","rating":4.5,"text":"nice"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/reviews", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestReviewsCreateReturnsNotFoundWhenStoreMissing(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB

	merchant := model.Merchant{Name: "Cafe"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	body := strings.NewReader(fmt.Sprintf(`{"merchantId":"%d","storeId":"99999","rating":4.5,"text":"nice"}`, merchant.ID))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/reviews", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestReviewsCreateReturnsUnprocessableWhenStoreMerchantMismatch(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB

	merchantA := model.Merchant{Name: "A"}
	if err := db.Create(&merchantA).Error; err != nil {
		t.Fatalf("failed to create merchantA: %v", err)
	}
	merchantB := model.Merchant{Name: "B"}
	if err := db.Create(&merchantB).Error; err != nil {
		t.Fatalf("failed to create merchantB: %v", err)
	}
	store := model.Store{MerchantID: merchantB.ID, Name: "B-Store"}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	body := strings.NewReader(fmt.Sprintf(`{"merchantId":"%d","storeId":"%d","rating":4.5,"text":"nice"}`, merchantA.ID, store.ID))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/reviews", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestReviewDetailIncludesMerchantFields(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	u := model.User{Role: "user", Status: 0}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	m := model.Merchant{Name: "Cafe", BusinessName: "Cafe Business", Address: "San Francisco"}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	review := model.Review{UserID: u.ID, MerchantID: m.ID, VenueID: m.ID, Rating: 4.5, Content: "ok"}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("failed to create review: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/reviews/%d", review.ID), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["businessName"] != "Cafe Business" {
		t.Fatalf("expected businessName Cafe Business, got %v", resp["businessName"])
	}
	if resp["location"] != "San Francisco" {
		t.Fatalf("expected location San Francisco, got %v", resp["location"])
	}
}

func TestReviewLikeAndComment(t *testing.T) {
	r, tok := setupAPITest(t)

	db := database.DB
	m := model.Merchant{Name: "Cafe"}
	_ = db.Create(&m).Error
	u := model.User{Role: "user", Status: 0}
	_ = db.Create(&u).Error
	review := model.Review{UserID: u.ID, MerchantID: m.ID, Rating: 4, Content: "ok"}
	_ = db.Create(&review).Error

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/reviews/%d/like", review.ID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	body := strings.NewReader(`{"text":"nice"}`)
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/reviews/%d/comments", review.ID), body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestStoreCouponCreateListAndValidate(t *testing.T) {
	r, tok := setupAPITest(t)

	db := database.DB
	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	ownerID := ownerAuth.UserID

	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Owner Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	createBody := strings.NewReader(`{"title":"Coffee Deal","description":"10% off","type":"discount","price":9.9,"total_quantity":5,"max_per_user":2}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/merchant/stores/%d/coupons", store.ID), createBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var created struct {
		Data model.Coupon `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create coupon response: %v", err)
	}
	if created.Data.StoreID == nil || *created.Data.StoreID != store.ID {
		t.Fatalf("expected coupon store_id %d, got %+v", store.ID, created.Data.StoreID)
	}
	if created.Data.MerchantID != merchant.ID {
		t.Fatalf("expected coupon merchant_id %d, got %d", merchant.ID, created.Data.MerchantID)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stores/%d/coupons", store.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d", w.Code)
	}
	var listed struct {
		Data []model.Coupon `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("failed to decode list coupons response: %v", err)
	}
	if len(listed.Data) != 1 || listed.Data[0].ID != created.Data.ID {
		t.Fatalf("expected listed coupon %d, got %+v", created.Data.ID, listed.Data)
	}

	validateBody := strings.NewReader(`{"quantity":1}`)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/coupons/%d/validate", created.Data.ID), validateBody)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected validate 200, got %d", w.Code)
	}
}

func TestCouponOrderPayAndMerchantRedeemFlow(t *testing.T) {
	r, ownerTok := setupAPITest(t)

	db := database.DB
	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	ownerID := ownerAuth.UserID

	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Owner Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Lunch Deal",
		Type:          "discount",
		Price:         12.5,
		TotalQuantity: 3,
		MaxPerUser:    2,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	buyer := model.User{Role: "user", Status: 0}
	if err := db.Create(&buyer).Error; err != nil {
		t.Fatalf("failed to create buyer: %v", err)
	}
	buyerTok := issueAPITestToken(t, buyer, "buyer@example.com")

	createOrderBody := strings.NewReader(fmt.Sprintf(`{"coupon_id":%d,"quantity":2}`, coupon.ID))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/orders", createOrderBody)
	req.Header.Set("Authorization", "Bearer "+buyerTok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected create order 201, got %d", w.Code)
	}
	var orderResp struct {
		Data model.Order `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &orderResp); err != nil {
		t.Fatalf("failed to decode create order response: %v", err)
	}
	if orderResp.Data.Status != "pending" {
		t.Fatalf("expected pending order status, got %s", orderResp.Data.Status)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/pay", orderResp.Data.ID), nil)
	req.Header.Set("Authorization", "Bearer "+buyerTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected pay 200, got %d", w.Code)
	}

	var paidOrder model.Order
	if err := db.First(&paidOrder, orderResp.Data.ID).Error; err != nil {
		t.Fatalf("failed to load paid order: %v", err)
	}
	if paidOrder.Status != "paid" {
		t.Fatalf("expected paid order status, got %s", paidOrder.Status)
	}

	var couponAfterPay model.Coupon
	if err := db.First(&couponAfterPay, coupon.ID).Error; err != nil {
		t.Fatalf("failed to load coupon: %v", err)
	}
	if couponAfterPay.ClaimedCount != 2 {
		t.Fatalf("expected claimed_count 2, got %d", couponAfterPay.ClaimedCount)
	}

	var vouchers []model.Voucher
	if err := db.Where("order_id = ?", paidOrder.ID).Order("id asc").Find(&vouchers).Error; err != nil {
		t.Fatalf("failed to load vouchers: %v", err)
	}
	if len(vouchers) != 2 {
		t.Fatalf("expected 2 vouchers, got %d", len(vouchers))
	}

	var payments []model.Payment
	if err := db.Where("order_id = ?", paidOrder.ID).Find(&payments).Error; err != nil {
		t.Fatalf("failed to load payments: %v", err)
	}
	if len(payments) != 1 || payments[0].Status != "success" {
		t.Fatalf("expected 1 success payment, got %+v", payments)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/pay", paidOrder.ID), nil)
	req.Header.Set("Authorization", "Bearer "+buyerTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected idempotent pay 200, got %d", w.Code)
	}
	if err := db.Where("order_id = ?", paidOrder.ID).Find(&vouchers).Error; err != nil {
		t.Fatalf("failed to reload vouchers: %v", err)
	}
	if len(vouchers) != 2 {
		t.Fatalf("expected still 2 vouchers after idempotent pay, got %d", len(vouchers))
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/merchant/vouchers/%d/redeem", vouchers[0].ID), nil)
	req.Header.Set("Authorization", "Bearer "+buyerTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected redeem forbidden 403, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/merchant/vouchers/%d/redeem", vouchers[0].ID), nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected owner redeem 200, got %d", w.Code)
	}

	var redeemed model.Voucher
	if err := db.First(&redeemed, vouchers[0].ID).Error; err != nil {
		t.Fatalf("failed to load redeemed voucher: %v", err)
	}
	if redeemed.Status != "used" {
		t.Fatalf("expected voucher used, got %s", redeemed.Status)
	}
}

func TestVoucherCreateAndList(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB
	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	merchant := model.Merchant{Name: "Free Coupon Merchant", UserID: &ownerAuth.UserID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Free Coupon Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Free Coupon",
		Type:          "free",
		TotalQuantity: 5,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	body := strings.NewReader(fmt.Sprintf(`{"couponId":"%d","userId":"999999","code":"FORGED"}`, coupon.ID))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/vouchers", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var created model.Voucher
	if err := db.Order("id DESC").First(&created).Error; err != nil {
		t.Fatalf("failed to load created voucher: %v", err)
	}
	if created.UserID != ownerAuth.UserID {
		t.Fatalf("expected authenticated owner %d, got %d", ownerAuth.UserID, created.UserID)
	}
	if created.Code == "FORGED" {
		t.Fatal("expected server-generated voucher code")
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/vouchers", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestVoucherDetailReturnsScanURLForOwner(t *testing.T) {
	r, ownerTok := setupAPITest(t)
	db := database.DB

	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}

	merchant := model.Merchant{Name: "Owner Merchant"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Owner Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Owner Coupon",
		Type:          "discount",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	voucher := model.Voucher{
		Code:       "OWNER-VOUCHER",
		ScanToken:  "scan-token-owner",
		CouponID:   coupon.ID,
		UserID:     ownerAuth.UserID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/vouchers/%d", voucher.ID), nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode voucher detail response: %v", err)
	}
	scanURL, _ := resp["scan_url"].(string)
	want := "https://merchant.revieu.test/merchant/vouchers/scan?t=scan-token-owner"
	if scanURL != want {
		t.Fatalf("unexpected scan_url: got %q want %q", scanURL, want)
	}
}

func TestVoucherDetailRejectsNonOwner(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	owner := model.User{Role: "user", Status: 0}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	other := model.User{Role: "user", Status: 0}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("failed to create other user: %v", err)
	}
	otherTok := issueAPITestToken(t, other, "other-voucher@example.com")

	merchant := model.Merchant{Name: "Voucher Merchant"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Voucher Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Voucher Coupon",
		Type:          "discount",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "NOT-OWNER-VOUCHER",
		ScanToken:  "scan-token-non-owner",
		CouponID:   coupon.ID,
		UserID:     owner.ID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/vouchers/%d", voucher.ID), nil)
	req.Header.Set("Authorization", "Bearer "+otherTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestVoucherByCodeRejectsNonOwner(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	owner := model.User{Role: "user", Status: 0}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	other := model.User{Role: "user", Status: 0}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("failed to create other user: %v", err)
	}
	otherTok := issueAPITestToken(t, other, "other-code@example.com")

	merchant := model.Merchant{Name: "ByCode Merchant"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "ByCode Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "ByCode Coupon",
		Type:          "discount",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "SECRET-CODE",
		ScanToken:  "scan-token-secret-code",
		CouponID:   coupon.ID,
		UserID:     owner.ID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/vouchers/code/SECRET-CODE", nil)
	req.Header.Set("Authorization", "Bearer "+otherTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMerchantVoucherScanPreviewFlow(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	merchantOwner := model.User{Role: "user", Status: 0}
	if err := db.Create(&merchantOwner).Error; err != nil {
		t.Fatalf("failed to create merchant owner: %v", err)
	}
	merchantOwnerTok := issueAPITestToken(t, merchantOwner, "merchant-preview@example.com")

	buyer := model.User{Role: "user", Status: 0}
	if err := db.Create(&buyer).Error; err != nil {
		t.Fatalf("failed to create buyer: %v", err)
	}

	merchant := model.Merchant{Name: "Preview Flow Merchant", UserID: &merchantOwner.ID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Preview Flow Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Preview Flow Coupon",
		Type:          "discount",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "PREVIEW-FLOW-VOUCHER",
		ScanToken:  "scan-token-preview-flow",
		CouponID:   coupon.ID,
		UserID:     buyer.ID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/merchant/vouchers/scan?t=scan-token-preview-flow", nil)
	req.Header.Set("Authorization", "Bearer "+merchantOwnerTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode scan preview response: %v", err)
	}
	if canRedeem, _ := resp["can_redeem"].(bool); !canRedeem {
		t.Fatalf("expected can_redeem=true, got %+v", resp["can_redeem"])
	}
	if voucherCode, _ := resp["voucher_code"].(string); voucherCode != voucher.Code {
		t.Fatalf("expected voucher_code=%q, got %q", voucher.Code, voucherCode)
	}
	if couponTitle, _ := resp["coupon_title"].(string); couponTitle != coupon.Title {
		t.Fatalf("expected coupon_title=%q, got %q", coupon.Title, couponTitle)
	}
}

func TestMerchantVoucherPreviewThenRedeemByTokenFlow(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	merchantOwner := model.User{Role: "user", Status: 0}
	if err := db.Create(&merchantOwner).Error; err != nil {
		t.Fatalf("failed to create merchant owner: %v", err)
	}
	merchantOwnerTok := issueAPITestToken(t, merchantOwner, "merchant-redeem-token@example.com")

	buyer := model.User{Role: "user", Status: 0}
	if err := db.Create(&buyer).Error; err != nil {
		t.Fatalf("failed to create buyer: %v", err)
	}

	merchant := model.Merchant{Name: "Redeem Token Flow Merchant", UserID: &merchantOwner.ID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Redeem Token Flow Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Redeem Token Flow Coupon",
		Type:          "discount",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "REDEEM-TOKEN-FLOW-VOUCHER",
		ScanToken:  "scan-token-redeem-flow",
		CouponID:   coupon.ID,
		UserID:     buyer.ID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/merchant/vouchers/scan?t=scan-token-redeem-flow", nil)
	req.Header.Set("Authorization", "Bearer "+merchantOwnerTok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected preview 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/merchant/vouchers/redeem-by-token", strings.NewReader(`{"scan_token":"scan-token-redeem-flow"}`))
	req.Header.Set("Authorization", "Bearer "+merchantOwnerTok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected redeem-by-token 200, got %d", w.Code)
	}

	var redeemed model.Voucher
	if err := db.First(&redeemed, voucher.ID).Error; err != nil {
		t.Fatalf("failed to load redeemed voucher: %v", err)
	}
	if redeemed.Status != "used" {
		t.Fatalf("expected voucher used, got %q", redeemed.Status)
	}
	if redeemed.RedeemedBy == nil || *redeemed.RedeemedBy != merchantOwner.ID {
		t.Fatalf("expected redeemed_by=%d, got %+v", merchantOwner.ID, redeemed.RedeemedBy)
	}

	var refreshedCoupon model.Coupon
	if err := db.First(&refreshedCoupon, coupon.ID).Error; err != nil {
		t.Fatalf("failed to load coupon: %v", err)
	}
	if refreshedCoupon.RedeemedCount != 1 {
		t.Fatalf("expected redeemed_count=1, got %d", refreshedCoupon.RedeemedCount)
	}
}

func TestPaymentsCreateAndDetail(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB
	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	order := model.Order{UserID: ownerAuth.UserID, Quantity: 1, TotalPrice: 10, Status: "pending"}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	body := strings.NewReader(fmt.Sprintf(`{"order_id":%d,"payment_method":"mock","amount":999999,"currency":"EUR","status":"success"}`, order.ID))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/payments", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var payment model.Payment
	if err := db.Order("id DESC").First(&payment).Error; err != nil {
		t.Fatalf("failed to load created payment: %v", err)
	}
	if payment.Amount != order.TotalPrice || payment.Currency != "USD" || payment.Status != "pending" {
		t.Fatalf("expected server-derived pending payment, got %+v", payment)
	}
}

func TestMediaUploadAndAnalysis(t *testing.T) {
	r, tok := setupAPITest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/media/uploads", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAISuggestions(t *testing.T) {
	r, tok := setupAPITest(t)

	// The endpoint now expects multipart/form-data and calls Gemini. Without a configured
	// API key the Gemini client is nil, so the service rejects the request with an upstream
	// error mapped to 502. The test asserts that the multipart contract and JWT middleware
	// are wired up (i.e. we do not get 400/401/404) rather than that Gemini is reachable.
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("text", "The coffee was great and the atmosphere was chill.")
	_ = mw.WriteField("merchantName", "Cafe")
	_ = mw.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/ai/reviews/suggestions", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 without a Gemini API key, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestAuthForgotPassword(t *testing.T) {
	r, _ := setupAPITest(t)

	body := strings.NewReader(`{"email":"user@example.com"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestStoreUpdateOwnStore(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB

	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	ownerID := ownerAuth.UserID

	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Old Store", City: "Old City"}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	body := strings.NewReader(`{"name":"Updated Store","city":"San Francisco"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/merchant/stores/%d", store.ID), body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var refreshed model.Store
	if err := db.First(&refreshed, store.ID).Error; err != nil {
		t.Fatalf("failed to load updated store: %v", err)
	}
	if refreshed.Name != "Updated Store" {
		t.Fatalf("unexpected updated name: got %q", refreshed.Name)
	}
	if refreshed.City != "San Francisco" {
		t.Fatalf("unexpected updated city: got %q", refreshed.City)
	}
}

func TestStoreMenuImages(t *testing.T) {
	r, tok := setupAPITest(t)
	db := database.DB

	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	ownerID := ownerAuth.UserID

	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Menu Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	body := strings.NewReader(`{"menu_images":["https://cdn.revieu.com/menu-1.jpg","https://cdn.revieu.com/menu-2.jpg"]}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/merchant/stores/%d", store.ID), body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected patch 200, got %d", w.Code)
	}

	var patchResp struct {
		Data model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &patchResp); err != nil {
		t.Fatalf("failed to decode patch response: %v", err)
	}
	if patchResp.Data.MenuImages != `["https://cdn.revieu.com/menu-1.jpg","https://cdn.revieu.com/menu-2.jpg"]` {
		t.Fatalf("unexpected patched menu images: %q", patchResp.Data.MenuImages)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stores/%d", store.ID), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d", w.Code)
	}

	var detailResp struct {
		Data model.Store `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("failed to decode detail response: %v", err)
	}
	if detailResp.Data.MenuImages != `["https://cdn.revieu.com/menu-1.jpg","https://cdn.revieu.com/menu-2.jpg"]` {
		t.Fatalf("unexpected detail menu images: %q", detailResp.Data.MenuImages)
	}
}

func TestStoreUpdateForbiddenForNonOwner(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	ownerID := ownerAuth.UserID

	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Protected Store"}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	other := model.User{Role: "user", Status: 0}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("failed to create other user: %v", err)
	}
	otherAuth := model.UserAuth{UserID: other.ID, IdentityType: "email", Identifier: "other@example.com"}
	if err := db.Create(&otherAuth).Error; err != nil {
		t.Fatalf("failed to create other auth: %v", err)
	}
	otherToken, err := token.New(config.JWTConfig{Secret: "test-secret", ExpireHour: 24}).GenerateToken(&other, &otherAuth)
	if err != nil {
		t.Fatalf("failed to generate token for other user: %v", err)
	}

	body := strings.NewReader(`{"name":"Hijacked Name"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/merchant/stores/%d", store.ID), body)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	var refreshed model.Store
	if err := db.First(&refreshed, store.ID).Error; err != nil {
		t.Fatalf("failed to load store: %v", err)
	}
	if refreshed.Name != "Protected Store" {
		t.Fatalf("store should not be updated by non-owner, got %q", refreshed.Name)
	}

}

func TestStoreUpdateUnauthorizedWithoutJWT(t *testing.T) {
	r, _ := setupAPITest(t)
	db := database.DB

	var ownerAuth model.UserAuth
	if err := db.Where("identifier = ?", "user@example.com").First(&ownerAuth).Error; err != nil {
		t.Fatalf("failed to load owner auth: %v", err)
	}
	ownerID := ownerAuth.UserID

	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Protected Store"}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	body := strings.NewReader(`{"name":"No Auth Update"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/merchant/stores/%d", store.ID), body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
