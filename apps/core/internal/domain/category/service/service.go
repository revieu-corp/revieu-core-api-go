package service

import (
	"context"
	"sort"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

type CategoryService struct {
	db *gorm.DB
}

func NewCategoryService(db *gorm.DB) *CategoryService {
	if db == nil {
		db = database.DB
	}
	return &CategoryService{db: db}
}

func (s *CategoryService) List(ctx context.Context) ([]model.Category, error) {
	var categories []model.Category
	if err := s.db.WithContext(ctx).Order("id asc").Find(&categories).Error; err != nil {
		return nil, err
	}

	byID := make(map[int64]struct{}, len(categories))
	for i := range categories {
		categories[i].Parent = nil
		categories[i].Children = nil
		byID[categories[i].ID] = struct{}{}
	}

	childrenByParent := make(map[int64][]model.Category, len(categories))
	roots := make([]model.Category, 0, len(categories))
	for _, category := range categories {
		if category.ParentID == nil {
			roots = append(roots, category)
			continue
		}
		_, parentExists := byID[*category.ParentID]
		if !parentExists || *category.ParentID == category.ID {
			// Keep orphaned rows visible instead of silently dropping them.
			roots = append(roots, category)
			continue
		}
		childrenByParent[*category.ParentID] = append(childrenByParent[*category.ParentID], category)
	}

	var build func(model.Category) model.Category
	build = func(category model.Category) model.Category {
		children := childrenByParent[category.ID]
		sort.SliceStable(children, func(i, j int) bool { return children[i].ID < children[j].ID })
		for _, child := range children {
			category.Children = append(category.Children, build(child))
		}
		return category
	}
	for i := range roots {
		roots[i] = build(roots[i])
	}
	sort.SliceStable(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
	return roots, nil
}
