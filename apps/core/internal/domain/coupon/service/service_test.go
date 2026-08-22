package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCouponTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Merchant{},
		&model.Store{},
		&model.Coupon{},
		&model.Dish{},
	); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	return db
}

func TestCouponServiceDeleteForStoreOwnerSoftDeleteAndIdempotent(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(801)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner user: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Published Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Delete Coupon",
		Type:          "cash",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        couponStatusActive,
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	if err := svc.DeleteForStore(context.Background(), ownerID, store.ID, coupon.ID); err != nil {
		t.Fatalf("delete coupon returned error: %v", err)
	}

	if err := svc.DeleteForStore(context.Background(), ownerID, store.ID, coupon.ID); err != nil {
		t.Fatalf("second delete should be idempotent, got error: %v", err)
	}

	var liveCoupon model.Coupon
	if err := db.First(&liveCoupon, coupon.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected scoped coupon query to hide deleted row, got err=%v", err)
	}

	var deletedCoupon model.Coupon
	if err := db.Unscoped().First(&deletedCoupon, coupon.ID).Error; err != nil {
		t.Fatalf("failed to query deleted coupon unscoped: %v", err)
	}
	if !deletedCoupon.DeletedAt.Valid {
		t.Fatalf("expected deleted coupon to have deleted_at set")
	}
}

func TestCouponServiceDeleteForStoreForbiddenForNonOwner(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(811)
	otherID := int64(812)
	for _, id := range []int64{ownerID, otherID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Owned Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Protected Coupon",
		Type:          "cash",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        couponStatusActive,
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	err := svc.DeleteForStore(context.Background(), otherID, store.ID, coupon.ID)
	if !errors.Is(err, ErrStoreForbidden) {
		t.Fatalf("expected ErrStoreForbidden, got %v", err)
	}
}

func TestCouponServiceDeleteForStoreRejectsCouponOutsideStore(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(821)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner user: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	storeA := model.Store{MerchantID: merchant.ID, Name: "Store A", Status: storeStatusPublished}
	storeB := model.Store{MerchantID: merchant.ID, Name: "Store B", Status: storeStatusPublished}
	if err := db.Create(&storeA).Error; err != nil {
		t.Fatalf("failed to create store A: %v", err)
	}
	if err := db.Create(&storeB).Error; err != nil {
		t.Fatalf("failed to create store B: %v", err)
	}
	storeBID := storeB.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeBID,
		Title:         "Store B Coupon",
		Type:          "cash",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        couponStatusActive,
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	err := svc.DeleteForStore(context.Background(), ownerID, storeA.ID, coupon.ID)
	if !errors.Is(err, ErrCouponNotFound) {
		t.Fatalf("expected ErrCouponNotFound for coupon-store mismatch, got %v", err)
	}
}

func TestCouponServiceCreateForStorePopulatesPricingAndDishes(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(901)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	dish := model.Dish{MerchantID: merchant.ID, Name: "Burger", OriginalPrice: 15, ImageURL: "https://example.com/burger.jpg", Status: "active"}
	if err := db.Create(&dish).Error; err != nil {
		t.Fatalf("failed to create dish: %v", err)
	}

	coupon, err := svc.CreateForStore(context.Background(), ownerID, store.ID, CreateStoreCouponInput{
		Title:              "20% OFF Burger",
		Type:               "percentage",
		CouponType:         "normal",
		OriginalPrice:      15,
		SalePrice:          12,
		DiscountPercentage: 20,
		TotalQuantity:      100,
		MaxPerUser:         1,
		DishIDs:            []int64{dish.ID},
	})
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}
	if coupon.OriginalPrice != 15 || coupon.SalePrice != 12 || coupon.DiscountPercentage != 20 {
		t.Fatalf("expected pricing fields to be populated, got %+v", coupon)
	}
	if coupon.CouponType != "normal" {
		t.Fatalf("expected coupon type normal, got %q", coupon.CouponType)
	}
	if coupon.DishIDs != fmt.Sprintf("[%d]", dish.ID) {
		t.Fatalf("expected dish_ids to encode [%d], got %q", dish.ID, coupon.DishIDs)
	}
	if coupon.ImageURL != dish.ImageURL {
		t.Fatalf("expected coupon image to default to the single associated dish's image, got %q", coupon.ImageURL)
	}
}

func TestCouponServiceCreateForStoreKeepsExplicitImageOverDishImage(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(902)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	dish := model.Dish{MerchantID: merchant.ID, Name: "Burger", OriginalPrice: 15, ImageURL: "https://example.com/burger.jpg", Status: "active"}
	if err := db.Create(&dish).Error; err != nil {
		t.Fatalf("failed to create dish: %v", err)
	}

	coupon, err := svc.CreateForStore(context.Background(), ownerID, store.ID, CreateStoreCouponInput{
		Title: "Custom Image Coupon", Type: "percentage", TotalQuantity: 10, MaxPerUser: 1,
		DishIDs: []int64{dish.ID}, ImageURL: "https://example.com/custom.jpg",
	})
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}
	if coupon.ImageURL != "https://example.com/custom.jpg" {
		t.Fatalf("expected explicit ImageURL to be kept, got %q", coupon.ImageURL)
	}
}

func TestComputeStatus(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name   string
		coupon model.Coupon
		want   string
	}{
		{"sold out beats everything else", model.Coupon{Status: couponStatusActive, TotalQuantity: 10, ClaimedCount: 10, ValidUntil: &future}, "sold_out"},
		{"expired when ValidUntil passed", model.Coupon{Status: couponStatusActive, TotalQuantity: 10, ClaimedCount: 1, ValidUntil: &past}, "expired"},
		{"scheduled when ValidFrom in future", model.Coupon{Status: couponStatusActive, TotalQuantity: 10, ClaimedCount: 1, ValidFrom: &future}, "scheduled"},
		{"active with no date bounds", model.Coupon{Status: couponStatusActive, TotalQuantity: 10, ClaimedCount: 1}, "active"},
		{"draft stays draft even if dates would say scheduled", model.Coupon{Status: "draft", TotalQuantity: 10, ClaimedCount: 0, ValidFrom: &future}, "draft"},
		{"disabled stays disabled even if dates would say active", model.Coupon{Status: "disabled", TotalQuantity: 10, ClaimedCount: 1}, "disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeStatus(tc.coupon, now); got != tc.want {
				t.Fatalf("ComputeStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCouponServiceListForMerchantIncludesAllStatuses(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(941)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	for _, status := range []string{"draft", couponStatusActive, "disabled"} {
		if err := db.Create(&model.Coupon{MerchantID: merchant.ID, StoreID: &storeID, Title: status + " coupon", Type: "cash", TotalQuantity: 1, MaxPerUser: 1, Status: status}).Error; err != nil {
			t.Fatalf("failed to seed %s coupon: %v", status, err)
		}
	}

	coupons, err := svc.ListForMerchant(context.Background(), ownerID, store.ID)
	if err != nil {
		t.Fatalf("list returned error: %v", err)
	}
	if len(coupons) != 3 {
		t.Fatalf("expected all 3 coupons regardless of status, got %d", len(coupons))
	}
}

func TestCouponServiceListForMerchantForbiddenForNonOwner(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(951)
	otherID := int64(952)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	if err := db.Create(&model.User{ID: otherID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create other: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	_, err := svc.ListForMerchant(context.Background(), otherID, store.ID)
	if !errors.Is(err, ErrStoreForbidden) {
		t.Fatalf("expected ErrStoreForbidden, got %v", err)
	}
}

func TestCouponServiceUpdateForStoreAppliesPartialFields(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(961)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{MerchantID: merchant.ID, StoreID: &storeID, Title: "Original", Type: "cash", TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusActive}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	newTitle := "Updated Title"
	newQuantity := 50
	updated, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{
		Title:         &newTitle,
		TotalQuantity: &newQuantity,
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}
	if updated.Title != newTitle || updated.TotalQuantity != newQuantity {
		t.Fatalf("expected updated fields to apply, got %+v", updated)
	}
}

func TestCouponServiceUpdateForStoreAppliesPriceField(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(962)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{MerchantID: merchant.ID, StoreID: &storeID, Title: "Price Test Coupon", Type: "cash", Price: 0, TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusActive}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	newPrice := 29.99
	updated, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{
		Price: &newPrice,
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}
	if updated.Price != newPrice {
		t.Fatalf("expected price to be updated to %f, got %f", newPrice, updated.Price)
	}

	// Re-fetch from database to ensure the change persisted
	var refreshed model.Coupon
	if err := db.First(&refreshed, coupon.ID).Error; err != nil {
		t.Fatalf("failed to re-fetch coupon: %v", err)
	}
	if refreshed.Price != newPrice {
		t.Fatalf("expected persisted price to be %f, got %f", newPrice, refreshed.Price)
	}
}

func TestCouponServiceUpdateForStoreForbiddenForNonOwner(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(971)
	otherID := int64(972)
	for _, id := range []int64{ownerID, otherID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{MerchantID: merchant.ID, StoreID: &storeID, Title: "Protected", Type: "cash", TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusActive}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	newTitle := "Hacked"
	_, err := svc.UpdateForStore(context.Background(), otherID, store.ID, coupon.ID, UpdateStoreCouponInput{Title: &newTitle})
	if !errors.Is(err, ErrStoreForbidden) {
		t.Fatalf("expected ErrStoreForbidden, got %v", err)
	}
}

func TestCouponServiceSetEnabledTogglesStatus(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(981)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{MerchantID: merchant.ID, StoreID: &storeID, Title: "Toggle Me", Type: "cash", TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusActive}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	disabled, err := svc.SetEnabled(context.Background(), ownerID, store.ID, coupon.ID, false)
	if err != nil {
		t.Fatalf("disable returned error: %v", err)
	}
	if disabled.Status != "disabled" {
		t.Fatalf("expected status disabled, got %q", disabled.Status)
	}

	enabled, err := svc.SetEnabled(context.Background(), ownerID, store.ID, coupon.ID, true)
	if err != nil {
		t.Fatalf("enable returned error: %v", err)
	}
	if enabled.Status != couponStatusActive {
		t.Fatalf("expected status active, got %q", enabled.Status)
	}
}

func TestCouponServiceUpdateForStoreRejectsInvertedDateRange(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(991)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID

	validFrom := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	validUntil := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Date Range Coupon",
		Type:          "cash",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        couponStatusActive,
		ValidFrom:     &validFrom,
		ValidUntil:    &validUntil,
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	// Try to update ValidFrom to after the existing ValidUntil
	newFrom := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	newFromPtr := &newFrom
	_, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{
		ValidFrom: &newFromPtr,
	})
	if !errors.Is(err, ErrInvalidCouponInput) {
		t.Fatalf("expected ErrInvalidCouponInput for inverted date range, got %v", err)
	}
}

func TestCouponServiceUpdateForStoreClearsCouponDates(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(993)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	validFrom := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	validUntil := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Clear Dates Coupon",
		Type:          "cash",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        couponStatusActive,
		ValidFrom:     &validFrom,
		ValidUntil:    &validUntil,
		ExpiryDate:    validUntil,
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	clearFrom := (*time.Time)(nil)
	clearUntil := (*time.Time)(nil)
	updated, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{
		ValidFrom:  &clearFrom,
		ValidUntil: &clearUntil,
	})
	if err != nil {
		t.Fatalf("clear dates returned error: %v", err)
	}
	if updated.ValidFrom != nil || updated.ValidUntil != nil {
		t.Fatalf("expected dates to be cleared, got from=%v until=%v", updated.ValidFrom, updated.ValidUntil)
	}

	var refreshed model.Coupon
	if err := db.First(&refreshed, coupon.ID).Error; err != nil {
		t.Fatalf("failed to re-fetch coupon: %v", err)
	}
	if refreshed.ValidFrom != nil || refreshed.ValidUntil != nil || !refreshed.ExpiryDate.IsZero() {
		t.Fatalf("expected persisted dates and expiry to be cleared, got from=%v until=%v expiry=%v", refreshed.ValidFrom, refreshed.ValidUntil, refreshed.ExpiryDate)
	}
}

func TestCouponServiceUpdateForStoreRejectsInvalidStatus(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(992)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{MerchantID: merchant.ID, StoreID: &storeID, Title: "Status Validation", Type: "cash", TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusActive}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	// Try to update status to a derived-only value
	invalidStatus := "sold_out"
	_, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{
		Status: &invalidStatus,
	})
	if !errors.Is(err, ErrInvalidCouponInput) {
		t.Fatalf("expected ErrInvalidCouponInput for invalid status, got %v", err)
	}
}

func TestCouponServiceCreateForStoreRejectsDerivedStatusAndForeignDish(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(993)
	otherOwnerID := int64(994)
	for _, id := range []int64{ownerID, otherOwnerID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}
	owner := model.Merchant{Name: "Owner", UserID: &ownerID}
	other := model.Merchant{Name: "Other", UserID: &otherOwnerID}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("failed to create owner merchant: %v", err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("failed to create other merchant: %v", err)
	}
	store := model.Store{MerchantID: owner.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	dish := model.Dish{MerchantID: other.ID, Name: "Foreign Dish", Status: "active"}
	if err := db.Create(&dish).Error; err != nil {
		t.Fatalf("failed to create foreign dish: %v", err)
	}

	derivedStatus := "sold_out"
	_, err := svc.CreateForStore(context.Background(), ownerID, store.ID, CreateStoreCouponInput{
		Title: "Invalid Status", Type: "cash", Status: derivedStatus, TotalQuantity: 10, MaxPerUser: 1,
	})
	if !errors.Is(err, ErrInvalidCouponInput) {
		t.Fatalf("expected derived status to be rejected, got %v", err)
	}

	_, err = svc.CreateForStore(context.Background(), ownerID, store.ID, CreateStoreCouponInput{
		Title: "Foreign Dish", Type: "cash", DishIDs: []int64{dish.ID}, TotalQuantity: 10, MaxPerUser: 1,
	})
	if !errors.Is(err, ErrInvalidCouponInput) {
		t.Fatalf("expected foreign dish to be rejected, got %v", err)
	}
}

func TestCouponServiceUpdateForStoreRejectsInvalidQuantityPricingAndDish(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(995)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID: merchant.ID, StoreID: &storeID, Title: "Coupon", Type: "cash",
		Price: 10, OriginalPrice: 20, SalePrice: 10, TotalQuantity: 10, ClaimedCount: 2,
		MaxPerUser: 1, Status: couponStatusActive,
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	zero := 0
	if _, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{TotalQuantity: &zero}); !errors.Is(err, ErrInvalidCouponInput) {
		t.Fatalf("expected zero total quantity to be rejected, got %v", err)
	}

	tooLow := 1
	if _, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{TotalQuantity: &tooLow}); !errors.Is(err, ErrInvalidCouponInput) {
		t.Fatalf("expected quantity below claimed count to be rejected, got %v", err)
	}

	badSale := 25.0
	if _, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{SalePrice: &badSale}); !errors.Is(err, ErrInvalidCouponInput) {
		t.Fatalf("expected sale price above original price to be rejected, got %v", err)
	}

	missingDish := int64(99999)
	if _, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{DishIDs: &[]int64{missingDish}}); !errors.Is(err, ErrInvalidCouponInput) {
		t.Fatalf("expected missing dish to be rejected, got %v", err)
	}
}

func TestCouponServiceListPublishedByStoreExcludesComputedSoldOut(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(996)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	merchant := model.Merchant{Name: "Owner", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	for _, coupon := range []model.Coupon{
		{MerchantID: merchant.ID, StoreID: &storeID, Title: "Available", Type: "cash", TotalQuantity: 10, ClaimedCount: 1, MaxPerUser: 1, Status: couponStatusActive},
		{MerchantID: merchant.ID, StoreID: &storeID, Title: "Sold Out", Type: "cash", TotalQuantity: 10, ClaimedCount: 10, MaxPerUser: 1, Status: couponStatusActive},
	} {
		if err := db.Create(&coupon).Error; err != nil {
			t.Fatalf("failed to create coupon %q: %v", coupon.Title, err)
		}
	}

	coupons, err := svc.ListPublishedByStore(context.Background(), store.ID)
	if err != nil {
		t.Fatalf("list published returned error: %v", err)
	}
	if len(coupons) != 1 || coupons[0].Title != "Available" {
		t.Fatalf("expected only available coupon, got %+v", coupons)
	}
}
