package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/store/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StoreService struct {
	db *gorm.DB
}

const (
	StoreStatusDraft     int16 = 0
	StoreStatusPublished int16 = 1
	StoreStatusHidden    int16 = 2

	defaultPublicListLimit   = 20
	defaultReviewsListLimit  = 20
	maxPublicListLimit       = 100
	maxStoreReviewsListLimit = 100
	defaultRadiusKM          = 20.0
)

var ErrUserNotFound = errors.New("user not found")
var ErrStoreNotFound = errors.New("store not found")
var ErrStoreForbidden = errors.New("store forbidden")
var ErrCategoryNotFound = errors.New("category not found")

func NewStoreService(db *gorm.DB) *StoreService {
	if db == nil {
		db = database.DB
	}
	return &StoreService{db: db}
}

func (s *StoreService) Create(ctx context.Context, userID int64, req dto.CreateStoreRequest) (*model.Store, error) {
	var user model.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		merchant = model.Merchant{
			UserID:             &userID,
			Name:               fmt.Sprintf("merchant-%d", userID),
			VerificationStatus: "unverified",
		}
		if err := s.db.WithContext(ctx).Create(&merchant).Error; err != nil {
			return nil, err
		}
	}

	imagesRaw, err := json.Marshal(req.Images)
	if err != nil {
		return nil, err
	}
	menuImagesRaw, err := json.Marshal(req.MenuImages)
	if err != nil {
		return nil, err
	}

	storeName := req.Name
	if storeName == "" {
		if merchant.BusinessName != "" {
			storeName = merchant.BusinessName
		} else if merchant.Name != "" {
			storeName = merchant.Name
		} else {
			storeName = fmt.Sprintf("store-%d", merchant.ID)
		}
	}

	store := model.Store{
		MerchantID:    merchant.ID,
		Name:          storeName,
		Description:   req.Description,
		Address:       req.Address,
		City:          req.City,
		State:         req.State,
		ZipCode:       req.ZipCode,
		Country:       req.Country,
		Phone:         req.Phone,
		Website:       req.Website,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
		CoverImageURL: req.CoverImageURL,
		Images:        string(imagesRaw),
		MenuImages:    string(menuImagesRaw),
		Status:        StoreStatusDraft,
	}

	categoryIDs, err := sanitizeCategoryIDs(req.CategoryIDs)
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&store).Error; err != nil {
			return err
		}
		if len(req.Hours) > 0 {
			hours := make([]model.StoreHour, 0, len(req.Hours))
			for _, h := range req.Hours {
				hours = append(hours, model.StoreHour{
					StoreID:   store.ID,
					DayOfWeek: h.DayOfWeek,
					OpenTime:  h.OpenTime,
					CloseTime: h.CloseTime,
					IsClosed:  h.IsClosed,
				})
			}
			if err := tx.Create(&hours).Error; err != nil {
				return err
			}
		}
		if len(categoryIDs) > 0 {
			if err := ensureCategoriesExist(tx, categoryIDs); err != nil {
				return err
			}
			links := make([]model.StoreCategory, 0, len(categoryIDs))
			for _, categoryID := range categoryIDs {
				links = append(links, model.StoreCategory{StoreID: store.ID, CategoryID: categoryID})
			}
			if err := tx.Create(&links).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.Merchant{}).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", merchant.ID).
			UpdateColumn("total_stores", gorm.Expr("total_stores + 1")).Error
	}); err != nil {
		return nil, err
	}

	return &store, nil
}

func (s *StoreService) ListPublished(ctx context.Context) ([]model.Store, error) {
	var stores []model.Store
	if err := s.db.WithContext(ctx).
		Where("status = ?", StoreStatusPublished).
		Order("id desc").
		Find(&stores).Error; err != nil {
		return nil, err
	}
	return stores, nil
}

func (s *StoreService) ListPublishedFiltered(ctx context.Context, query dto.StoreListQuery) ([]model.Store, *int64, error) {
	limit := sanitizeLimit(query.Limit, defaultPublicListLimit, maxPublicListLimit)

	dbQuery := s.db.WithContext(ctx).
		Model(&model.Store{}).
		Where("stores.status = ?", StoreStatusPublished)

	if query.Category != nil && strings.TrimSpace(*query.Category) != "" {
		dbQuery = dbQuery.
			Joins("JOIN store_categories sc ON sc.store_id = stores.id").
			Joins("JOIN categories c ON c.id = sc.category_id")

		category := strings.TrimSpace(*query.Category)
		if categoryID, err := strconv.ParseInt(category, 10, 64); err == nil {
			dbQuery = dbQuery.Where("c.id = ?", categoryID)
		} else {
			dbQuery = dbQuery.Where("LOWER(c.name) = LOWER(?)", category)
		}
	}

	if query.Rating != nil {
		dbQuery = dbQuery.Where("stores.avg_rating >= ?", *query.Rating)
	}

	if query.Lat != nil && query.Lng != nil {
		radiusKM := defaultRadiusKM
		if query.RadiusKM != nil && *query.RadiusKM > 0 {
			radiusKM = *query.RadiusKM
		}
		latDelta := radiusKM / 111.0
		lngDelta := radiusKM / 111.0
		dbQuery = dbQuery.
			Where("stores.latitude BETWEEN ? AND ?", *query.Lat-latDelta, *query.Lat+latDelta).
			Where("stores.longitude BETWEEN ? AND ?", *query.Lng-lngDelta, *query.Lng+lngDelta)
	}

	if query.Cursor != nil {
		dbQuery = dbQuery.Where("stores.id < ?", *query.Cursor)
	}

	var stores []model.Store
	if err := dbQuery.
		Distinct("stores.*").
		Order("stores.id desc").
		Limit(limit + 1).
		Find(&stores).Error; err != nil {
		return nil, nil, err
	}

	pageItems, cursor := sliceStorePage(stores, limit)
	return pageItems, cursor, nil
}

func (s *StoreService) DetailPublished(ctx context.Context, storeID int64) (*model.Store, error) {
	var store model.Store
	if err := s.db.WithContext(ctx).
		Preload("Hours").
		Preload("Categories").
		Where("id = ? AND status = ?", storeID, StoreStatusPublished).
		First(&store).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}
	return &store, nil
}

func (s *StoreService) ReviewsPublished(ctx context.Context, storeID int64) ([]model.Review, error) {
	if _, err := s.DetailPublished(ctx, storeID); err != nil {
		return nil, err
	}
	var reviews []model.Review
	if err := s.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Order("id desc").
		Find(&reviews).Error; err != nil {
		return nil, err
	}
	return reviews, nil
}

func (s *StoreService) ReviewsPublishedPaginated(ctx context.Context, storeID int64, query dto.StoreReviewListQuery) ([]model.Review, *int64, error) {
	if _, err := s.DetailPublished(ctx, storeID); err != nil {
		return nil, nil, err
	}

	limit := sanitizeLimit(query.Limit, defaultReviewsListLimit, maxStoreReviewsListLimit)

	dbQuery := s.db.WithContext(ctx).
		Model(&model.Review{}).
		Preload("User").
		Preload("User.Profile").
		Where("store_id = ?", storeID)

	if query.Cursor != nil {
		dbQuery = dbQuery.Where("reviews.id < ?", *query.Cursor)
	}

	var reviews []model.Review
	if err := dbQuery.
		Order("reviews.id desc").
		Limit(limit + 1).
		Find(&reviews).Error; err != nil {
		return nil, nil, err
	}
	pageItems, cursor := sliceReviewPage(reviews, limit)
	return pageItems, cursor, nil
}

func (s *StoreService) HoursPublished(ctx context.Context, storeID int64) ([]model.StoreHour, error) {
	if _, err := s.DetailPublished(ctx, storeID); err != nil {
		return nil, err
	}
	var hours []model.StoreHour
	if err := s.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Order("day_of_week asc").
		Find(&hours).Error; err != nil {
		return nil, err
	}
	return hours, nil
}

func (s *StoreService) ListMine(ctx context.Context, userID int64, limit *int) ([]model.Store, error) {
	dbQuery := s.db.WithContext(ctx).
		Model(&model.Store{}).
		Preload("Hours").
		Joins("JOIN merchants ON merchants.id = stores.merchant_id").
		Where("merchants.user_id = ?", userID).
		Order("stores.id desc")

	if limit != nil {
		dbQuery = dbQuery.Limit(sanitizeLimit(limit, defaultPublicListLimit, maxPublicListLimit))
	}

	var stores []model.Store
	if err := dbQuery.Find(&stores).Error; err != nil {
		return nil, err
	}
	return stores, nil
}

func (s *StoreService) Activate(ctx context.Context, userID, storeID int64) error {
	return s.updateStatusOwned(ctx, userID, storeID, StoreStatusPublished)
}

func (s *StoreService) Deactivate(ctx context.Context, userID, storeID int64) error {
	return s.updateStatusOwned(ctx, userID, storeID, StoreStatusHidden)
}

func (s *StoreService) Delete(ctx context.Context, userID, storeID int64) error {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStoreForbidden
		}
		return err
	}

	var store model.Store
	if err := s.db.WithContext(ctx).Unscoped().First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStoreNotFound
		}
		return err
	}
	if store.MerchantID != merchant.ID {
		return ErrStoreForbidden
	}
	if store.DeletedAt.Valid {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("store_id = ?", storeID).Delete(&model.Coupon{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", storeID).Delete(&model.Store{}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Merchant{}).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", merchant.ID).
			UpdateColumn("total_stores", gorm.Expr("CASE WHEN total_stores > 0 THEN total_stores - 1 ELSE 0 END")).Error
	})
}

func (s *StoreService) updateStatusOwned(ctx context.Context, userID, storeID int64, toStatus int16) error {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStoreForbidden
		}
		return err
	}

	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStoreNotFound
		}
		return err
	}
	if store.MerchantID != merchant.ID {
		return ErrStoreForbidden
	}
	if store.Status == toStatus {
		return nil
	}
	return s.db.WithContext(ctx).
		Model(&model.Store{}).
		Where("id = ?", storeID).
		UpdateColumn("status", toStatus).Error
}

func (s *StoreService) Update(ctx context.Context, userID, storeID int64, req dto.UpdateStoreRequest) (*model.Store, error) {
	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}

	var merchant model.Merchant
	if err := s.db.WithContext(ctx).First(&merchant, store.MerchantID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}
	if merchant.UserID == nil || *merchant.UserID != userID {
		return nil, ErrStoreForbidden
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Address != nil {
		updates["address"] = *req.Address
	}
	if req.City != nil {
		updates["city"] = *req.City
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.ZipCode != nil {
		updates["zip_code"] = *req.ZipCode
	}
	if req.Country != nil {
		updates["country"] = *req.Country
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}
	if req.Latitude != nil {
		updates["latitude"] = *req.Latitude
	}
	if req.Longitude != nil {
		updates["longitude"] = *req.Longitude
	}
	if req.CoverImageURL != nil {
		updates["cover_image_url"] = *req.CoverImageURL
	}
	if req.Images != nil {
		imagesRaw, err := json.Marshal(*req.Images)
		if err != nil {
			return nil, err
		}
		updates["images"] = string(imagesRaw)
	}
	if req.MenuImages != nil {
		menuImagesRaw, err := json.Marshal(*req.MenuImages)
		if err != nil {
			return nil, err
		}
		updates["menu_images"] = string(menuImagesRaw)
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var categoryIDs []int64
		if req.CategoryIDs != nil {
			normalized, err := sanitizeCategoryIDs(*req.CategoryIDs)
			if err != nil {
				return err
			}
			categoryIDs = normalized
			if len(categoryIDs) > 0 {
				if err := ensureCategoriesExist(tx, categoryIDs); err != nil {
					return err
				}
			}
		}

		if len(updates) > 0 {
			if err := tx.Model(&model.Store{}).
				Where("id = ?", storeID).
				Updates(updates).Error; err != nil {
				return err
			}
		}

		if req.Hours != nil {
			if err := tx.Where("store_id = ?", storeID).Delete(&model.StoreHour{}).Error; err != nil {
				return err
			}
			if len(*req.Hours) > 0 {
				hours := make([]model.StoreHour, 0, len(*req.Hours))
				for _, h := range *req.Hours {
					hours = append(hours, model.StoreHour{
						StoreID:   storeID,
						DayOfWeek: h.DayOfWeek,
						OpenTime:  h.OpenTime,
						CloseTime: h.CloseTime,
						IsClosed:  h.IsClosed,
					})
				}
				if err := tx.Create(&hours).Error; err != nil {
					return err
				}
			}
		}

		if req.CategoryIDs != nil {
			if err := tx.Where("store_id = ?", storeID).Delete(&model.StoreCategory{}).Error; err != nil {
				return err
			}
			if len(categoryIDs) > 0 {
				links := make([]model.StoreCategory, 0, len(categoryIDs))
				for _, categoryID := range categoryIDs {
					links = append(links, model.StoreCategory{
						StoreID:    storeID,
						CategoryID: categoryID,
					})
				}
				if err := tx.Create(&links).Error; err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	var updated model.Store
	if err := s.db.WithContext(ctx).Preload("Hours").Preload("Categories").First(&updated, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}
	return &updated, nil
}

func sanitizeLimit(raw *int, defaultLimit, maxLimit int) int {
	if raw == nil || *raw <= 0 {
		return defaultLimit
	}
	if *raw > maxLimit {
		return maxLimit
	}
	return *raw
}

func sliceStorePage(items []model.Store, limit int) ([]model.Store, *int64) {
	if len(items) <= limit {
		return items, nil
	}
	items = items[:limit]
	if len(items) == 0 {
		return items, nil
	}
	lastID := items[len(items)-1].ID
	return items, &lastID
}

func sliceReviewPage(items []model.Review, limit int) ([]model.Review, *int64) {
	if len(items) <= limit {
		return items, nil
	}
	items = items[:limit]
	if len(items) == 0 {
		return items, nil
	}
	lastID := items[len(items)-1].ID
	return items, &lastID
}

func sanitizeCategoryIDs(raw []int64) ([]int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[int64]struct{}, len(raw))
	normalized := make([]int64, 0, len(raw))
	for _, id := range raw {
		if id <= 0 {
			return nil, ErrCategoryNotFound
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized, nil
}

func ensureCategoriesExist(tx *gorm.DB, categoryIDs []int64) error {
	if len(categoryIDs) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&model.Category{}).Where("id IN ?", categoryIDs).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(categoryIDs)) {
		return ErrCategoryNotFound
	}
	return nil
}
