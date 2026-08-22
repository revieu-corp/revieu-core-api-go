package service

import (
	"context"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestListBuildsCategoryHierarchy(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewCategoryService(db)

	root := model.Category{Name: "Food", IconURL: "food.svg"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("failed to create root category: %v", err)
	}
	child := model.Category{Name: "Cafe", ParentID: &root.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("failed to create child category: %v", err)
	}
	grandchild := model.Category{Name: "Bakery", ParentID: &child.ID}
	if err := db.Create(&grandchild).Error; err != nil {
		t.Fatalf("failed to create grandchild category: %v", err)
	}
	orphanParentID := int64(99999)
	orphan := model.Category{Name: "Other", ParentID: &orphanParentID}
	if err := db.Create(&orphan).Error; err != nil {
		t.Fatalf("failed to create orphan category: %v", err)
	}

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list categories failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected root and orphan categories, got %d", len(got))
	}
	if got[0].Name != "Food" || len(got[0].Children) != 1 {
		t.Fatalf("expected Food to contain one child, got %#v", got[0])
	}
	if got[0].Children[0].Name != "Cafe" || len(got[0].Children[0].Children) != 1 {
		t.Fatalf("expected Cafe to contain Bakery, got %#v", got[0].Children[0])
	}
	if got[0].Children[0].Children[0].Name != "Bakery" {
		t.Fatalf("expected Bakery as grandchild, got %#v", got[0].Children[0].Children[0])
	}
	if got[1].Name != "Other" {
		t.Fatalf("expected orphan category to remain visible, got %#v", got[1])
	}
}

func TestListReturnsEmptySliceWhenNoCategoriesExist(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewCategoryService(db)

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list categories failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil empty category slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected no categories, got %d", len(got))
	}
}
