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

func TestDishServiceCRUDAndOwnership(t *testing.T) {
	db := setupDishTestDB(t)
	svc := NewDishService(db)
	ownerID := int64(901)
	otherID := int64(902)
	if err := db.Create(&model.User{ID: ownerID, Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{ID: otherID, Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	merchant := model.Merchant{Name: "Dish Merchant", UserID: &ownerID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatal(err)
	}

	dish, err := svc.Create(context.Background(), ownerID, UpsertDishInput{Name: "Noodles", OriginalPrice: 12.5, Category: "entree"})
	if err != nil {
		t.Fatalf("create dish returned error: %v", err)
	}
	list, err := svc.ListMine(context.Background(), ownerID)
	if err != nil || len(list) != 1 || list[0].ID != dish.ID {
		t.Fatalf("unexpected dish list: %+v err=%v", list, err)
	}
	if _, err := svc.SetStatus(context.Background(), otherID, dish.ID, DishStatusDisabled); !errors.Is(err, ErrMerchantNotFound) {
		t.Fatalf("expected non-owner to be unable to access dish, got %v", err)
	}
	if _, err := svc.SetStatus(context.Background(), ownerID, dish.ID, DishStatusDisabled); err != nil {
		t.Fatalf("disable dish returned error: %v", err)
	}
	if err := svc.Delete(context.Background(), ownerID, dish.ID); err != nil {
		t.Fatalf("delete dish returned error: %v", err)
	}
	if _, err := svc.Update(context.Background(), ownerID, dish.ID, UpdateDishInput{}); !errors.Is(err, ErrDishNotFound) {
		t.Fatalf("expected deleted dish to be hidden, got %v", err)
	}
}
