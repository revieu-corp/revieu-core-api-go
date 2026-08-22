package service

import (
	"context"
	"errors"
	"testing"

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
		&model.Dish{},
		&model.Coupon{},
		&model.OperationalAuditLog{},
	); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	return db
}

func TestCouponServiceMerchantCRUDAndStatus(t *testing.T) {
	db := setupCouponTestDB(t)
	svc := NewCouponService(db)

	ownerID := int64(831)
	if err := db.Create(&model.User{ID: ownerID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create owner user: %v", err)
	}
	merchant := model.Merchant{Name: "Coupon Owner", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Coupon Store", Status: storeStatusPublished}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	dish := model.Dish{MerchantID: merchant.ID, Name: "House Dish", Status: "active"}
	if err := db.Create(&dish).Error; err != nil {
		t.Fatalf("failed to create dish: %v", err)
	}
	original := 10.0
	sale := 7.0
	coupon, err := svc.CreateForStore(context.Background(), ownerID, store.ID, CreateStoreCouponInput{
		Title: "Owner Coupon", Type: "percentage", CouponType: "normal", Price: sale,
		OriginalPrice: &original, SalePrice: &sale, DishIDs: []int64{dish.ID},
		TotalQuantity: 10, MaxPerUser: 1, Status: couponStatusDraft,
	})
	if err != nil {
		t.Fatalf("create coupon returned error: %v", err)
	}
	if coupon.Status != couponStatusDraft || coupon.OriginalPrice != original || coupon.SalePrice != sale {
		t.Fatalf("unexpected created coupon: %+v", coupon)
	}

	listed, err := svc.ListForStore(context.Background(), ownerID, store.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != coupon.ID {
		t.Fatalf("unexpected merchant coupon list: len=%d err=%v", len(listed), err)
	}
	newTitle := "Updated Coupon"
	updated, err := svc.UpdateForStore(context.Background(), ownerID, store.ID, coupon.ID, UpdateStoreCouponInput{Title: &newTitle})
	if err != nil || updated.Title != newTitle {
		t.Fatalf("update coupon returned %+v, err=%v", updated, err)
	}
	active, err := svc.SetStatusForStore(context.Background(), ownerID, store.ID, coupon.ID, couponStatusActive)
	if err != nil || active.Status != couponStatusActive {
		t.Fatalf("enable coupon returned %+v, err=%v", active, err)
	}

	var audit model.OperationalAuditLog
	if err := db.Where("action = ? AND target_id = ? AND result = ?", "coupon.activate", coupon.ID, "success").First(&audit).Error; err != nil {
		t.Fatalf("expected successful coupon activation audit: %v", err)
	}
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
