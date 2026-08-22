package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
)

func TestCouponServiceMerchantLifecycleEnforcesOwnershipAndVerifiedActivation(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(1501)
	otherID := int64(1502)
	if err := db.Create(&[]model.User{
		{ID: ownerID, Role: "merchant", Status: 0},
		{ID: otherID, Role: "merchant", Status: 0},
	}).Error; err != nil {
		t.Fatalf("failed to create merchant users: %v", err)
	}
	owner := model.Merchant{Name: "Owner", UserID: &ownerID, VerificationStatus: "verified"}
	other := model.Merchant{Name: "Other", UserID: &otherID, VerificationStatus: "verified"}
	if err := db.Create(&[]model.Merchant{owner, other}).Error; err != nil {
		t.Fatalf("failed to create merchants: %v", err)
	}
	if err := db.First(&owner, "user_id = ?", ownerID).Error; err != nil {
		t.Fatalf("failed to reload owner merchant: %v", err)
	}
	if err := db.First(&other, "user_id = ?", otherID).Error; err != nil {
		t.Fatalf("failed to reload other merchant: %v", err)
	}
	store := model.Store{MerchantID: owner.ID, Name: "Owner Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	otherStore := model.Store{MerchantID: other.ID, Name: "Other Store", Status: storeStatusPublished}
	if err := db.Create(&otherStore).Error; err != nil {
		t.Fatalf("failed to create other store: %v", err)
	}
	coupon := model.Coupon{MerchantID: owner.ID, StoreID: &store.ID, Title: "Draft", Type: "cash", TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusDraft}
	secondCoupon := model.Coupon{MerchantID: owner.ID, StoreID: &store.ID, Title: "Second draft", Type: "cash", TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusDraft}
	otherCoupon := model.Coupon{MerchantID: other.ID, StoreID: &otherStore.ID, Title: "Other", Type: "cash", TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusDraft}
	for _, value := range []*model.Coupon{&coupon, &secondCoupon, &otherCoupon} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("failed to create coupon: %v", err)
		}
	}

	page, err := svc.ListForMerchant(context.Background(), ownerID, ListMerchantCouponsQuery{Status: couponStatusDraft, Limit: 1})
	if err != nil {
		t.Fatalf("merchant coupon list returned error: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].MerchantID != owner.ID || page.Cursor == nil {
		t.Fatalf("expected one owned draft and a next cursor, got %+v", page)
	}

	newTitle := "Updated draft"
	updated, err := svc.UpdateForMerchant(context.Background(), ownerID, coupon.ID, UpdateStoreCouponInput{Title: &newTitle})
	if err != nil || updated.Title != newTitle {
		t.Fatalf("merchant coupon update returned %+v, err=%v", updated, err)
	}
	if _, err := svc.UpdateForMerchant(context.Background(), otherID, coupon.ID, UpdateStoreCouponInput{Title: &newTitle}); !errors.Is(err, ErrCouponNotFound) {
		t.Fatalf("expected non-owner update to be hidden as not found, got %v", err)
	}

	active, err := svc.SetStatusForMerchant(context.Background(), ownerID, coupon.ID, couponStatusActive)
	if err != nil || active.Status != couponStatusActive {
		t.Fatalf("merchant activation returned %+v, err=%v", active, err)
	}
	if _, err := svc.SetStatusForMerchant(context.Background(), ownerID, coupon.ID, couponStatusActive); err != nil {
		t.Fatalf("repeated activation should be idempotent, got %v", err)
	}
	disabled, err := svc.SetStatusForMerchant(context.Background(), ownerID, coupon.ID, couponStatusDisabled)
	if err != nil || disabled.Status != couponStatusDisabled {
		t.Fatalf("merchant deactivation returned %+v, err=%v", disabled, err)
	}
	if _, err := svc.SetStatusForMerchant(context.Background(), ownerID, coupon.ID, couponStatusDisabled); err != nil {
		t.Fatalf("repeated deactivation should be idempotent, got %v", err)
	}
	if _, err := svc.Validate(context.Background(), coupon.ID, ValidateInput{}); !errors.Is(err, ErrCouponInactive) {
		t.Fatalf("expected disabled coupon to be excluded from validation, got %v", err)
	}
}

func TestCouponServiceMerchantLifecycleRejectsUnverifiedAndExpiredActivation(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	pendingID := int64(1511)
	if err := db.Create(&model.User{ID: pendingID, Role: "merchant", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create pending merchant user: %v", err)
	}
	pendingMerchant := model.Merchant{Name: "Pending", UserID: &pendingID, VerificationStatus: "pending"}
	if err := db.Create(&pendingMerchant).Error; err != nil {
		t.Fatalf("failed to create pending merchant: %v", err)
	}
	pendingStore := model.Store{MerchantID: pendingMerchant.ID, Name: "Pending Store", Status: storeStatusPublished}
	if err := db.Create(&pendingStore).Error; err != nil {
		t.Fatalf("failed to create pending store: %v", err)
	}
	pendingCoupon := model.Coupon{MerchantID: pendingMerchant.ID, StoreID: &pendingStore.ID, Title: "Pending coupon", Type: "cash", TotalQuantity: 5, MaxPerUser: 1, Status: couponStatusDraft}
	if err := db.Create(&pendingCoupon).Error; err != nil {
		t.Fatalf("failed to create pending coupon: %v", err)
	}
	if _, err := svc.SetStatusForMerchant(context.Background(), pendingID, pendingCoupon.ID, couponStatusActive); !errors.Is(err, ErrMerchantForbidden) {
		t.Fatalf("expected pending merchant activation to be forbidden, got %v", err)
	}
	if _, err := svc.SetStatusForMerchant(context.Background(), pendingID, pendingCoupon.ID, couponStatusDisabled); err != nil {
		t.Fatalf("pending merchant should be able to deactivate, got %v", err)
	}

	verifiedID := int64(1512)
	if err := db.Create(&model.User{ID: verifiedID, Role: "merchant", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create verified merchant user: %v", err)
	}
	verifiedMerchant := model.Merchant{Name: "Verified", UserID: &verifiedID, VerificationStatus: "verified"}
	if err := db.Create(&verifiedMerchant).Error; err != nil {
		t.Fatalf("failed to create verified merchant: %v", err)
	}
	verifiedStore := model.Store{MerchantID: verifiedMerchant.ID, Name: "Verified Store", Status: storeStatusPublished}
	if err := db.Create(&verifiedStore).Error; err != nil {
		t.Fatalf("failed to create verified store: %v", err)
	}
	expiredAt := time.Now().Add(-time.Hour)
	expiredCoupon := model.Coupon{MerchantID: verifiedMerchant.ID, StoreID: &verifiedStore.ID, Title: "Expired", Type: "cash", TotalQuantity: 5, MaxPerUser: 1, Status: couponStatusDraft, ValidUntil: &expiredAt}
	if err := db.Create(&expiredCoupon).Error; err != nil {
		t.Fatalf("failed to create expired coupon: %v", err)
	}
	if _, err := svc.SetStatusForMerchant(context.Background(), verifiedID, expiredCoupon.ID, couponStatusActive); !errors.Is(err, ErrCouponExpired) {
		t.Fatalf("expected expired activation to fail, got %v", err)
	}

	regularID := int64(1513)
	if err := db.Create(&model.User{ID: regularID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create regular user: %v", err)
	}
	if _, err := svc.ListForMerchant(context.Background(), regularID, ListMerchantCouponsQuery{}); !errors.Is(err, ErrMerchantForbidden) {
		t.Fatalf("expected regular user coupon list to be forbidden, got %v", err)
	}
}
