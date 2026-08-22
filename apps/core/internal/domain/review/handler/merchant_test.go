package handler

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestMerchantReviewHandlerReplyAndDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	svc := service.NewReviewService(db)
	h := NewMerchantReviewHandler(svc)

	owner := model.User{Role: "user", Status: 0}
	customer := model.User{Role: "user", Status: 0}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	merchant := model.Merchant{Name: "Handler Cafe", UserID: &owner.ID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	review := model.Review{
		UserID:     customer.ID,
		VenueID:    merchant.ID,
		MerchantID: merchant.ID,
		Rating:     5,
		Content:    "Great service.",
		VisitDate:  time.Now().UTC(),
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create review: %v", err)
	}

	replyRecorder := httptest.NewRecorder()
	replyCtx, _ := gin.CreateTestContext(replyRecorder)
	reviewID := strconv.FormatInt(review.ID, 10)
	replyCtx.Params = gin.Params{{Key: "id", Value: reviewID}}
	replyCtx.Set("user_id", owner.ID)
	replyCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/merchant/reviews/"+reviewID+"/reply", strings.NewReader(`{"text":"Thanks for visiting."}`))
	replyCtx.Request.Header.Set("Content-Type", "application/json")
	h.Reply(replyCtx)
	if replyRecorder.Code != http.StatusOK {
		t.Fatalf("expected reply 200, got %d: %s", replyRecorder.Code, replyRecorder.Body.String())
	}
	if !strings.Contains(replyRecorder.Body.String(), "Thanks for visiting.") {
		t.Fatalf("reply response missing text: %s", replyRecorder.Body.String())
	}

	deleteRecorder := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRecorder)
	deleteCtx.Params = gin.Params{{Key: "id", Value: reviewID}}
	deleteCtx.Set("user_id", owner.ID)
	deleteCtx.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/merchant/reviews/"+reviewID, nil)
	h.Delete(deleteCtx)
	if deleteRecorder.Code != http.StatusOK || !strings.Contains(deleteRecorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected delete 200 ok, got %d: %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
}

func TestMerchantReviewHandlerRejectsWrongOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	svc := service.NewReviewService(db)
	h := NewMerchantReviewHandler(svc)

	owner := model.User{Role: "user", Status: 0}
	other := model.User{Role: "user", Status: 0}
	customer := model.User{Role: "user", Status: 0}
	for _, user := range []*model.User{&owner, &other, &customer} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	merchant := model.Merchant{Name: "Protected Cafe", UserID: &owner.ID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	review := model.Review{
		UserID:     customer.ID,
		VenueID:    merchant.ID,
		MerchantID: merchant.ID,
		Rating:     3,
		Content:    "Okay.",
		VisitDate:  time.Now().UTC(),
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create review: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	reviewID := strconv.FormatInt(review.ID, 10)
	c.Params = gin.Params{{Key: "id", Value: reviewID}}
	c.Set("user_id", other.ID)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/merchant/reviews/"+reviewID+"/reply", strings.NewReader(`{"text":"Not allowed."}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Reply(c)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
