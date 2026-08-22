package service

import (
	"context"
	"errors"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDishTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Merchant{}, &model.Dish{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	return db
}

func createOwnedMerchant(t *testing.T, db *gorm.DB, userID int64) model.Merchant {
	t.Helper()
	if err := db.Create(&model.User{ID: userID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	merchant := model.Merchant{Name: "Owner Merchant", UserID: &userID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	return merchant
}

func TestDishServiceCreateAndListMine(t *testing.T) {
	db := setupDishTestDB(t)
	svc := NewDishService(db)
	ownerID := int64(101)
	createOwnedMerchant(t, db, ownerID)

	dish, err := svc.Create(context.Background(), ownerID, CreateDishInput{
		Name:          "Beef Burger",
		Description:   "Juicy beef patty",
		OriginalPrice: 12.5,
		Category:      "Burgers",
		ImageURL:      "https://example.com/burger.jpg",
	})
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}
	if dish.Status != "active" {
		t.Fatalf("expected new dish to default to active, got %q", dish.Status)
	}

	dishes, err := svc.ListMine(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list returned error: %v", err)
	}
	if len(dishes) != 1 || dishes[0].ID != dish.ID {
		t.Fatalf("expected list to contain the created dish, got %+v", dishes)
	}
}

func TestDishServiceCreateRejectsInvalidInput(t *testing.T) {
	db := setupDishTestDB(t)
	svc := NewDishService(db)
	ownerID := int64(102)
	createOwnedMerchant(t, db, ownerID)

	_, err := svc.Create(context.Background(), ownerID, CreateDishInput{Name: "", OriginalPrice: 5})
	if !errors.Is(err, ErrInvalidDishInput) {
		t.Fatalf("expected ErrInvalidDishInput for empty name, got %v", err)
	}

	_, err = svc.Create(context.Background(), ownerID, CreateDishInput{Name: "Fries", OriginalPrice: -1})
	if !errors.Is(err, ErrInvalidDishInput) {
		t.Fatalf("expected ErrInvalidDishInput for negative price, got %v", err)
	}
}

func TestDishServiceUpdateForbiddenForNonOwner(t *testing.T) {
	db := setupDishTestDB(t)
	svc := NewDishService(db)
	ownerID := int64(111)
	otherID := int64(112)
	createOwnedMerchant(t, db, ownerID)
	if err := db.Create(&model.User{ID: otherID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create other user: %v", err)
	}

	dish, err := svc.Create(context.Background(), ownerID, CreateDishInput{Name: "Fries", OriginalPrice: 4})
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}

	newName := "Hacked Fries"
	_, err = svc.Update(context.Background(), otherID, dish.ID, UpdateDishInput{Name: &newName})
	if !errors.Is(err, ErrDishForbidden) {
		t.Fatalf("expected ErrDishForbidden, got %v", err)
	}
}

func TestDishServiceSetStatusEnableDisable(t *testing.T) {
	db := setupDishTestDB(t)
	svc := NewDishService(db)
	ownerID := int64(121)
	createOwnedMerchant(t, db, ownerID)

	dish, err := svc.Create(context.Background(), ownerID, CreateDishInput{Name: "Shake", OriginalPrice: 6})
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}

	updated, err := svc.SetStatus(context.Background(), ownerID, dish.ID, "disabled")
	if err != nil {
		t.Fatalf("set status returned error: %v", err)
	}
	if updated.Status != "disabled" {
		t.Fatalf("expected status disabled, got %q", updated.Status)
	}
}

func TestDishServiceDeleteSoftDeletesAndIsIdempotent(t *testing.T) {
	db := setupDishTestDB(t)
	svc := NewDishService(db)
	ownerID := int64(131)
	createOwnedMerchant(t, db, ownerID)

	dish, err := svc.Create(context.Background(), ownerID, CreateDishInput{Name: "Salad", OriginalPrice: 8})
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}

	if err := svc.Delete(context.Background(), ownerID, dish.ID); err != nil {
		t.Fatalf("delete returned error: %v", err)
	}
	if err := svc.Delete(context.Background(), ownerID, dish.ID); err != nil {
		t.Fatalf("second delete should be idempotent, got error: %v", err)
	}

	var liveDish model.Dish
	if err := db.First(&liveDish, dish.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected scoped query to hide deleted dish, got err=%v", err)
	}
}
