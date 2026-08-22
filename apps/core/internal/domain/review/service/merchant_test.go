package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestMerchantReviewWorkflowPersistsReplyAndArchive(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewReviewService(db)

	merchantUser := model.User{Role: "user", Status: 0}
	customer := model.User{Role: "user", Status: 0}
	if err := db.Create(&merchantUser).Error; err != nil {
		t.Fatalf("create merchant user: %v", err)
	}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	if err := db.Create(&model.UserProfile{UserID: customer.ID, Nickname: "Jamie Customer"}).Error; err != nil {
		t.Fatalf("create customer profile: %v", err)
	}

	merchant := model.Merchant{Name: "Review Cafe", UserID: &merchantUser.ID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	review := model.Review{
		UserID:     customer.ID,
		VenueID:    merchant.ID,
		MerchantID: merchant.ID,
		Rating:     2,
		Content:    "The wait was too long.",
		VisitDate:  time.Now().UTC(),
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create review: %v", err)
	}

	items, err := svc.ListMerchantReviews(context.Background(), merchantUser.ID)
	if err != nil {
		t.Fatalf("list merchant reviews: %v", err)
	}
	if len(items) != 1 || items[0].CustomerName != "Jamie Customer" || items[0].HasReply {
		t.Fatalf("unexpected initial review list: %#v", items)
	}

	item, err := svc.ReplyToMerchantReview(context.Background(), merchantUser.ID, review.ID, "Thank you for the feedback.")
	if err != nil {
		t.Fatalf("create merchant reply: %v", err)
	}
	if !item.HasReply || item.ReplyText != "Thank you for the feedback." {
		t.Fatalf("unexpected reply response: %#v", item)
	}

	if _, err := svc.ReplyToMerchantReview(context.Background(), merchantUser.ID, review.ID, "We have improved our staffing."); err != nil {
		t.Fatalf("update merchant reply: %v", err)
	}
	var replies []model.ReviewComment
	if err := db.Where("review_id = ? AND is_merchant_reply = ?", review.ID, true).Find(&replies).Error; err != nil {
		t.Fatalf("load merchant replies: %v", err)
	}
	if len(replies) != 1 || replies[0].Content != "We have improved our staffing." {
		t.Fatalf("expected one updated reply, got %#v", replies)
	}

	items, err = svc.ListMerchantReviews(context.Background(), merchantUser.ID)
	if err != nil {
		t.Fatalf("reload merchant reviews: %v", err)
	}
	if len(items) != 1 || !items[0].HasReply || items[0].ReplyText != "We have improved our staffing." {
		t.Fatalf("reply did not survive reload: %#v", items)
	}

	if err := svc.DeleteMerchantReview(context.Background(), merchantUser.ID, review.ID); err != nil {
		t.Fatalf("archive merchant review: %v", err)
	}
	items, err = svc.ListMerchantReviews(context.Background(), merchantUser.ID)
	if err != nil {
		t.Fatalf("list after archive: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("archived review remained active: %#v", items)
	}

	var archived model.Review
	if err := db.First(&archived, review.ID).Error; err != nil {
		t.Fatalf("load archived review: %v", err)
	}
	if archived.Status != reviewStatusArchived {
		t.Fatalf("expected archived status %d, got %d", reviewStatusArchived, archived.Status)
	}
}

func TestMerchantReviewMutationsEnforceOwnershipAndValidation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewReviewService(db)

	owner := model.User{Role: "user", Status: 0}
	other := model.User{Role: "user", Status: 0}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Cafe", UserID: &owner.ID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	review := model.Review{
		UserID:     other.ID,
		VenueID:    merchant.ID,
		MerchantID: merchant.ID,
		Rating:     4,
		Content:    "Good.",
		VisitDate:  time.Now().UTC(),
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("create review: %v", err)
	}

	if _, err := svc.ReplyToMerchantReview(context.Background(), other.ID, review.ID, "Not yours"); !errors.Is(err, ErrMerchantNotFound) {
		t.Fatalf("expected non-merchant rejection, got %v", err)
	}
	if _, err := svc.ReplyToMerchantReview(context.Background(), owner.ID, review.ID, "   "); !errors.Is(err, ErrInvalidReply) {
		t.Fatalf("expected blank reply validation, got %v", err)
	}
	if _, err := svc.ReplyToMerchantReview(context.Background(), owner.ID, review.ID, string(make([]rune, 501))); !errors.Is(err, ErrInvalidReply) {
		t.Fatalf("expected long reply validation, got %v", err)
	}

	secondMerchantUser := model.User{Role: "user", Status: 0}
	if err := db.Create(&secondMerchantUser).Error; err != nil {
		t.Fatalf("create second merchant user: %v", err)
	}
	if err := db.Create(&model.Merchant{Name: "Other Cafe", UserID: &secondMerchantUser.ID}).Error; err != nil {
		t.Fatalf("create second merchant: %v", err)
	}
	if _, err := svc.ReplyToMerchantReview(context.Background(), secondMerchantUser.ID, review.ID, "Wrong owner"); !errors.Is(err, ErrReviewForbidden) {
		t.Fatalf("expected reply ownership rejection, got %v", err)
	}
	if err := svc.DeleteMerchantReview(context.Background(), secondMerchantUser.ID, review.ID); !errors.Is(err, ErrReviewForbidden) {
		t.Fatalf("expected delete ownership rejection, got %v", err)
	}
}
