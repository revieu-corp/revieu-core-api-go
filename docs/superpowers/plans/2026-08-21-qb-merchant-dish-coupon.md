# QB Merchant, Dish & Coupon Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a merchant create a store from scratch, manage a menu of dishes, and create/edit/enable/disable real (backend-persisted) coupons tied to those dishes — closing the gaps behind revieu-core-api-go issues #224–#227.

**Architecture:** Backend: extend the existing `store`/`coupon` domains and add a new `dish` domain in `revieu-core-api-go`, following the established dto/service/handler/routes package shape. Frontend: extend the existing `revieu-web/src/features/merchant` app (no new "QB" namespace) — add a store-creation branch to `StoreProfile.tsx`, a new dish-management page, and new coupon form/list components that replace the currently-fake `CouponManager` local-state flow.

**Tech Stack:** Go + Gin + GORM + PostgreSQL (backend, `apps/core`), React + TypeScript + axios (frontend, `revieu-web`). Backend tests use the repo's `sqlite :memory:` service-level convention (see `coupon/service/service_test.go`).

**Spec:** `docs/superpowers/specs/2026-08-21-qb-merchant-dish-coupon-design.md`

## Global Constraints

- Backend module root for all Go commands: `/home/paul2/workspace/repos/revieu-core-api-go/apps/core` (has its own `go.mod`).
- Frontend root for all npm/vitest commands: `/home/paul2/workspace/repos/revieu-web`.
- No new "QB" frontend namespace — all new UI lives under `revieu-web/src/features/merchant/`.
- The auto-verify-on-create behavior is gated behind reading `AUTO_VERIFY_NEW_MERCHANTS` from the process environment (default disabled) — never an unconditional change to verification behavior.
- Coupon `sold_out`/`expired`/`scheduled`/`active` status is computed at read time, not stored by a background job. Only `draft`/`active`/`disabled` are stored values.
- Follow existing code style exactly: this codebase's newer domain files (`config.go`, `coupon/service/service.go`) use dense single-line struct/func bodies with semicolons in places — but the `coupon` and `store` domains you are extending use conventional multi-line Go formatting. Match the file you are editing, not a repo-wide style.
- Every backend task must leave `go build ./...` and `go test ./...` (run from `apps/core`) green before moving to the next task.

---

## Task 1: Demo-only auto-verify flag in `StoreService`

**Files:**
- Modify: `apps/core/internal/domain/store/service/service.go`
- Test: `apps/core/internal/domain/store/service/service_test.go`

**Interfaces:**
- Produces: `autoVerifyNewMerchantsEnabled() bool` (unexported package helper), used internally by `Create` and `updateStatusOwned`. No public API changes.

- [ ] **Step 1: Write the failing tests**

Add to the bottom of `apps/core/internal/domain/store/service/service_test.go` (match the existing `setupServiceTestDB`/table style already in that file — use whatever the file's existing DB-setup helper is named; if unsure, run `grep -n "^func setup" apps/core/internal/domain/store/service/service_test.go` first and reuse that helper rather than inventing a new one):

```go
func TestStoreServiceCreateAutoVerifiesMerchantWhenFlagEnabled(t *testing.T) {
	t.Setenv("AUTO_VERIFY_NEW_MERCHANTS", "true")
	db := setupServiceTestDB(t) // use the file's real helper name
	svc := NewStoreService(db)

	userID := int64(9001)
	if err := db.Create(&model.User{ID: userID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if _, err := svc.Create(context.Background(), userID, dto.CreateStoreRequest{Name: "Flag On Store"}); err != nil {
		t.Fatalf("create returned error: %v", err)
	}

	var merchant model.Merchant
	if err := db.Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		t.Fatalf("failed to load merchant: %v", err)
	}
	if merchant.VerificationStatus != "verified" || merchant.Status != 0 {
		t.Fatalf("expected merchant to be auto-verified, got status=%d verification_status=%q", merchant.Status, merchant.VerificationStatus)
	}
}

func TestStoreServiceCreateDoesNotAutoVerifyByDefault(t *testing.T) {
	db := setupServiceTestDB(t) // flag left unset
	svc := NewStoreService(db)

	userID := int64(9002)
	if err := db.Create(&model.User{ID: userID, Role: "user", Status: 0}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if _, err := svc.Create(context.Background(), userID, dto.CreateStoreRequest{Name: "Flag Off Store"}); err != nil {
		t.Fatalf("create returned error: %v", err)
	}

	var merchant model.Merchant
	if err := db.Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		t.Fatalf("failed to load merchant: %v", err)
	}
	if merchant.VerificationStatus == "verified" {
		t.Fatalf("expected merchant to remain unverified when flag is off, got %q", merchant.VerificationStatus)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/core && go test ./internal/domain/store/service/... -run TestStoreServiceCreate -v`
Expected: FAIL — `AUTO_VERIFY_NEW_MERCHANTS`/auto-verify not implemented, merchant stays `unverified` in both cases.

- [ ] **Step 3: Implement the flag and hook it into `Create` and `updateStatusOwned`**

In `apps/core/internal/domain/store/service/service.go`, add `"os"` and `"strconv"` to imports if not already present (`strconv` is already imported), then add near the other unexported helpers:

```go
func autoVerifyNewMerchantsEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("AUTO_VERIFY_NEW_MERCHANTS"))
	return err == nil && enabled
}

// verifyMerchantIfEnabled marks a merchant as active/verified when the
// AUTO_VERIFY_NEW_MERCHANTS demo flag is on. It never runs otherwise, so
// normal merchant verification review is unaffected in every other
// environment. See docs/superpowers/specs/2026-08-21-qb-merchant-dish-coupon-design.md.
func (s *StoreService) verifyMerchantIfEnabled(ctx context.Context, merchantID int64) error {
	if !autoVerifyNewMerchantsEnabled() {
		return nil
	}
	return s.db.WithContext(ctx).
		Model(&model.Merchant{}).
		Where("id = ?", merchantID).
		Updates(map[string]interface{}{"status": 0, "verification_status": "verified"}).Error
}
```

Then call it at the end of `Create` (after the store row is successfully created — find the `if err := s.db.WithContext(ctx).Create(&store).Error; err != nil { return nil, err }` line near the end of `Create` and add right after it, before the final `return &store, nil`):

```go
	if err := s.verifyMerchantIfEnabled(ctx, merchant.ID); err != nil {
		return nil, err
	}
```

And in `updateStatusOwned`, right after the successful `UpdateColumn("status", toStatus)` call, only when publishing:

```go
	if err := s.db.WithContext(ctx).
		Model(&model.Store{}).
		Where("id = ?", storeID).
		UpdateColumn("status", toStatus).Error; err != nil {
		return err
	}
	if toStatus == StoreStatusPublished {
		return s.verifyMerchantIfEnabled(ctx, merchant.ID)
	}
	return nil
```

(This replaces the existing single-line `return s.db.WithContext(ctx)...UpdateColumn(...).Error` at the end of `updateStatusOwned` — read the current function body first since line numbers will have shifted from earlier exploration.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/core && go test ./internal/domain/store/service/... -v`
Expected: PASS for the two new tests and all pre-existing store service tests.

- [ ] **Step 5: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/domain/store/service/service.go apps/core/internal/domain/store/service/service_test.go
git commit -m "feat(store): auto-verify merchant on create/publish behind AUTO_VERIFY_NEW_MERCHANTS flag

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 2: `Dish` model

**Files:**
- Create: `apps/core/internal/model/dish.go`
- Modify: `apps/core/internal/model/migrate.go`

**Interfaces:**
- Produces: `model.Dish{ID, MerchantID, Name, ImageURL, Description, OriginalPrice, Category, Status, CreatedAt, UpdatedAt, DeletedAt}`, `(*Dish) TableName() string`.

- [ ] **Step 1: Create the model**

```go
package model

import (
	"time"

	"gorm.io/gorm"
)

// Dish is a menu item owned by a merchant. It is merchant-private in this
// pass (no public read endpoint) — its ImageURL is copied onto a Coupon's
// ImageURL when a coupon is created against it.
type Dish struct {
	ID            int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	MerchantID    int64          `gorm:"not null;index" json:"merchant_id"`
	Name          string         `gorm:"type:varchar(100);not null" json:"name"`
	ImageURL      string         `gorm:"type:varchar(255)" json:"image_url"`
	Description   string         `gorm:"type:text" json:"description"`
	OriginalPrice float64        `gorm:"type:numeric(10,2);not null;default:0" json:"original_price"`
	Category      string         `gorm:"type:varchar(50)" json:"category"`
	Status        string         `gorm:"type:varchar(20);not null;default:'active'" json:"status"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (d *Dish) TableName() string {
	return "dishes"
}
```

- [ ] **Step 2: Register it in `model.All()`**

In `apps/core/internal/model/migrate.go`, add `&Dish{}` under the "Merchant & Store" section (right after `&Store{}` and `&StoreHour{}`):

```go
		// Merchant & Store
		&Merchant{},
		&Category{},
		&StoreCategory{},
		&Store{},
		&StoreHour{},
		&Dish{},
```

- [ ] **Step 3: Verify it compiles**

Run: `cd apps/core && go build ./...`
Expected: succeeds with no errors.

- [ ] **Step 4: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/model/dish.go apps/core/internal/model/migrate.go
git commit -m "feat(model): add Dish model (#225)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 3: `dish` service

**Files:**
- Create: `apps/core/internal/domain/dish/service/service.go`
- Test: `apps/core/internal/domain/dish/service/service_test.go`

**Interfaces:**
- Consumes: `model.Merchant{UserID}`, `model.Dish` (Task 2).
- Produces: `service.NewDishService(db *gorm.DB) *DishService`; `(*DishService) Create(ctx, userID int64, input CreateDishInput) (*model.Dish, error)`; `(*DishService) Update(ctx, userID, dishID int64, input UpdateDishInput) (*model.Dish, error)`; `(*DishService) Delete(ctx, userID, dishID int64) error`; `(*DishService) ListMine(ctx, userID int64) ([]model.Dish, error)`; `(*DishService) SetStatus(ctx, userID, dishID int64, status string) (*model.Dish, error)`; errors `ErrDishNotFound`, `ErrDishForbidden`, `ErrInvalidDishInput`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/core && go test ./internal/domain/dish/... -v`
Expected: FAIL — package `service` does not exist yet.

- [ ] **Step 3: Implement the service**

```go
package service

import (
	"context"
	"errors"
	"strings"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

var (
	ErrDishNotFound     = errors.New("dish not found")
	ErrDishForbidden    = errors.New("dish forbidden")
	ErrInvalidDishInput = errors.New("invalid dish input")
)

const (
	DishStatusActive   = "active"
	DishStatusDisabled = "disabled"
)

type CreateDishInput struct {
	Name          string
	ImageURL      string
	Description   string
	OriginalPrice float64
	Category      string
}

type UpdateDishInput struct {
	Name          *string
	ImageURL      *string
	Description   *string
	OriginalPrice *float64
	Category      *string
}

type DishService struct {
	db *gorm.DB
}

func NewDishService(db *gorm.DB) *DishService {
	if db == nil {
		db = database.DB
	}
	return &DishService{db: db}
}

func (s *DishService) resolveMerchant(ctx context.Context, userID int64) (*model.Merchant, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDishForbidden
		}
		return nil, err
	}
	return &merchant, nil
}

func (s *DishService) Create(ctx context.Context, userID int64, input CreateDishInput) (*model.Dish, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || input.OriginalPrice < 0 {
		return nil, ErrInvalidDishInput
	}
	merchant, err := s.resolveMerchant(ctx, userID)
	if err != nil {
		return nil, err
	}

	dish := model.Dish{
		MerchantID:    merchant.ID,
		Name:          name,
		ImageURL:      input.ImageURL,
		Description:   input.Description,
		OriginalPrice: input.OriginalPrice,
		Category:      input.Category,
		Status:        DishStatusActive,
	}
	if err := s.db.WithContext(ctx).Create(&dish).Error; err != nil {
		return nil, err
	}
	return &dish, nil
}

func (s *DishService) ListMine(ctx context.Context, userID int64) ([]model.Dish, error) {
	merchant, err := s.resolveMerchant(ctx, userID)
	if err != nil {
		return nil, err
	}
	var dishes []model.Dish
	if err := s.db.WithContext(ctx).
		Where("merchant_id = ?", merchant.ID).
		Order("id desc").
		Find(&dishes).Error; err != nil {
		return nil, err
	}
	return dishes, nil
}

func (s *DishService) loadOwnedDish(ctx context.Context, userID, dishID int64) (*model.Dish, error) {
	merchant, err := s.resolveMerchant(ctx, userID)
	if err != nil {
		return nil, err
	}
	var dish model.Dish
	if err := s.db.WithContext(ctx).First(&dish, dishID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDishNotFound
		}
		return nil, err
	}
	if dish.MerchantID != merchant.ID {
		return nil, ErrDishForbidden
	}
	return &dish, nil
}

func (s *DishService) Update(ctx context.Context, userID, dishID int64, input UpdateDishInput) (*model.Dish, error) {
	dish, err := s.loadOwnedDish(ctx, userID, dishID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" {
			return nil, ErrInvalidDishInput
		}
		updates["name"] = trimmed
	}
	if input.ImageURL != nil {
		updates["image_url"] = *input.ImageURL
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.OriginalPrice != nil {
		if *input.OriginalPrice < 0 {
			return nil, ErrInvalidDishInput
		}
		updates["original_price"] = *input.OriginalPrice
	}
	if input.Category != nil {
		updates["category"] = *input.Category
	}
	if len(updates) == 0 {
		return dish, nil
	}
	if err := s.db.WithContext(ctx).Model(&model.Dish{}).Where("id = ?", dishID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.loadOwnedDish(ctx, userID, dishID)
}

func (s *DishService) SetStatus(ctx context.Context, userID, dishID int64, status string) (*model.Dish, error) {
	if status != DishStatusActive && status != DishStatusDisabled {
		return nil, ErrInvalidDishInput
	}
	if _, err := s.loadOwnedDish(ctx, userID, dishID); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.Dish{}).Where("id = ?", dishID).UpdateColumn("status", status).Error; err != nil {
		return nil, err
	}
	return s.loadOwnedDish(ctx, userID, dishID)
}

func (s *DishService) Delete(ctx context.Context, userID, dishID int64) error {
	merchant, err := s.resolveMerchant(ctx, userID)
	if err != nil {
		return err
	}
	var dish model.Dish
	if err := s.db.WithContext(ctx).Unscoped().First(&dish, dishID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDishNotFound
		}
		return err
	}
	if dish.MerchantID != merchant.ID {
		return ErrDishForbidden
	}
	if dish.DeletedAt.Valid {
		return nil
	}
	return s.db.WithContext(ctx).Where("id = ?", dishID).Delete(&model.Dish{}).Error
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/core && go test ./internal/domain/dish/... -v`
Expected: PASS for all six tests.

- [ ] **Step 5: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/domain/dish/service/
git commit -m "feat(dish): add dish service with CRUD + enable/disable (#225)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 4: `dish` handler + routes

**Files:**
- Create: `apps/core/internal/domain/dish/handler/dish.go`
- Create: `apps/core/internal/domain/dish/routes.go`
- Modify: `apps/core/internal/router/router.go`

**Interfaces:**
- Consumes: `dishService.NewDishService`, `service.CreateDishInput`/`UpdateDishInput` (Task 3).
- Produces: registers `POST/GET /merchant/dishes`, `PATCH/DELETE /merchant/dishes/:id`, `POST /merchant/dishes/:id/enable`, `POST /merchant/dishes/:id/disable`.

- [ ] **Step 1: Write the handler**

```go
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish/service"
)

type DishHandler struct {
	svc *service.DishService
}

func NewDishHandler(svc *service.DishService) *DishHandler {
	if svc == nil {
		svc = service.NewDishService(nil)
	}
	return &DishHandler{svc: svc}
}

type UpsertDishRequest struct {
	Name          string  `json:"name"`
	ImageURL      string  `json:"image_url"`
	Description   string  `json:"description"`
	OriginalPrice float64 `json:"original_price"`
	Category      string  `json:"category"`
}

type UpdateDishRequest struct {
	Name          *string  `json:"name"`
	ImageURL      *string  `json:"image_url"`
	Description   *string  `json:"description"`
	OriginalPrice *float64 `json:"original_price"`
	Category      *string  `json:"category"`
}

func dishErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrDishNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrDishForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrInvalidDishInput):
		return http.StatusBadRequest, "invalid dish input"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// CreateDish godoc
// @Summary Create dish
// @Tags dish
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /merchant/dishes [post]
func (h *DishHandler) Create(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req UpsertDishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dish, err := h.svc.Create(c.Request.Context(), userID, service.CreateDishInput{
		Name: req.Name, ImageURL: req.ImageURL, Description: req.Description,
		OriginalPrice: req.OriginalPrice, Category: req.Category,
	})
	if err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": dish})
}

// ListMine godoc
// @Summary List my dishes
// @Tags dish
// @Produce json
// @Security BearerAuth
// @Router /merchant/dishes [get]
func (h *DishHandler) ListMine(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishes, err := h.svc.ListMine(c.Request.Context(), userID)
	if err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dishes})
}

func parseDishID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dish id"})
		return 0, false
	}
	return id, true
}

// Update godoc
// @Summary Update dish
// @Tags dish
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /merchant/dishes/{id} [patch]
func (h *DishHandler) Update(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, ok := parseDishID(c)
	if !ok {
		return
	}
	var req UpdateDishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dish, err := h.svc.Update(c.Request.Context(), userID, dishID, service.UpdateDishInput{
		Name: req.Name, ImageURL: req.ImageURL, Description: req.Description,
		OriginalPrice: req.OriginalPrice, Category: req.Category,
	})
	if err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dish})
}

// Delete godoc
// @Summary Delete dish
// @Tags dish
// @Produce json
// @Security BearerAuth
// @Router /merchant/dishes/{id} [delete]
func (h *DishHandler) Delete(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, ok := parseDishID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, dishID); err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *DishHandler) setStatus(c *gin.Context, status string) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	dishID, ok := parseDishID(c)
	if !ok {
		return
	}
	dish, err := h.svc.SetStatus(c.Request.Context(), userID, dishID, status)
	if err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dish})
}

// Enable godoc
// @Summary Enable dish
// @Tags dish
// @Produce json
// @Security BearerAuth
// @Router /merchant/dishes/{id}/enable [post]
func (h *DishHandler) Enable(c *gin.Context) { h.setStatus(c, service.DishStatusActive) }

// Disable godoc
// @Summary Disable dish
// @Tags dish
// @Produce json
// @Security BearerAuth
// @Router /merchant/dishes/{id}/disable [post]
func (h *DishHandler) Disable(c *gin.Context) { h.setStatus(c, service.DishStatusDisabled) }
```

Note: `setStatus` shadows the parameter name `status` with the local var from `dishErrorStatus` — rename the parameter to `targetStatus` to avoid the shadow:

```go
func (h *DishHandler) setStatus(c *gin.Context, targetStatus string) {
	...
	dish, err := h.svc.SetStatus(c.Request.Context(), userID, dishID, targetStatus)
	if err != nil {
		status, msg := dishErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": dish})
}
```

- [ ] **Step 2: Write the routes file**

```go
package dish

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish/service"
)

// RegisterRoutes registers merchant-private dish management routes.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewDishService(nil)
	h := handler.NewDishHandler(svc)

	dishes := r.Group("/merchant/dishes", authorization.JWTAuth(cfg.JWT))
	{
		dishes.POST("", h.Create)
		dishes.GET("", h.ListMine)
		dishes.PATCH("/:id", h.Update)
		dishes.DELETE("/:id", h.Delete)
		dishes.POST("/:id/enable", h.Enable)
		dishes.POST("/:id/disable", h.Disable)
	}
}
```

- [ ] **Step 3: Wire it into the router**

Domain routes are registered in `apps/core/internal/router/router.go`. Add the import in alphabetical order with the other `internal/domain/*` imports:

```go
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish"
```

(it sorts between `"internal/domain/coupon"` and `"internal/domain/feed"`).

Then add the registration call next to `merchant.RegisterRoutes(api, cfg)` (the router group variable in this file is named `api`):

```go
	dish.RegisterRoutes(api, cfg)
```

- [ ] **Step 4: Verify it builds and the full test suite still passes**

Run: `cd apps/core && go build ./... && go test ./...`
Expected: builds clean, all tests pass (including Task 3's).

- [ ] **Step 5: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/domain/dish/handler/ apps/core/internal/domain/dish/routes.go apps/core/internal/router/
git commit -m "feat(dish): expose dish CRUD + enable/disable routes (#225)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 5: Coupon model — add `DishIDs`

**Files:**
- Modify: `apps/core/internal/model/coupon.go`

**Interfaces:**
- Produces: `model.Coupon.DishIDs string` (jsonb-backed, holds a JSON array of dish IDs; empty/`"[]"` means "applies to all dishes").

- [ ] **Step 1: Add the field**

In `apps/core/internal/model/coupon.go`, add a field after `ImageURL`:

```go
	ImageURL           string         `gorm:"type:varchar(255)" json:"image_url"`
	DishIDs            string         `gorm:"type:jsonb;default:'[]'" json:"dish_ids"`
```

- [ ] **Step 2: Verify it builds**

Run: `cd apps/core && go build ./...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/model/coupon.go
git commit -m "feat(model): add Coupon.DishIDs for coupon-to-dish association (#226)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 6: Extend `CreateForStore` with pricing, dish association, and coupon type

**Files:**
- Modify: `apps/core/internal/domain/coupon/service/service.go`
- Test: `apps/core/internal/domain/coupon/service/service_test.go`

**Interfaces:**
- Consumes: `model.Dish` (Task 2) for the "copy dish image when unset" behavior.
- Produces: `CreateStoreCouponInput` gains `OriginalPrice float64`, `SalePrice float64`, `DiscountPercentage float64`, `CouponType string`, `DishIDs []int64`, `ImageURL string`. `CreateForStore` populates these on the created `model.Coupon` and marshals `DishIDs` to `Coupon.DishIDs` (JSON string).

- [ ] **Step 1: Write the failing tests**

Append to `apps/core/internal/domain/coupon/service/service_test.go`:

```go
func TestCouponServiceCreateForStorePopulatesPricingAndDishes(t *testing.T) {
	db := setupCouponTestDB(t)
	if err := db.AutoMigrate(&model.Dish{}); err != nil {
		t.Fatalf("failed to migrate dish table: %v", err)
	}
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
	if err := db.AutoMigrate(&model.Dish{}); err != nil {
		t.Fatalf("failed to migrate dish table: %v", err)
	}
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
```

Also update `setupCouponTestDB` in the same file to migrate `&model.Dish{}` unconditionally instead of the per-test `db.AutoMigrate(&model.Dish{})` calls above, to keep the two new tests consistent with the rest of the file — do this by adding `&model.Dish{}` to the `AutoMigrate` list inside `setupCouponTestDB` and then **removing** the two inline `db.AutoMigrate(&model.Dish{})` lines from the tests above before running them.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/core && go test ./internal/domain/coupon/service/... -run TestCouponServiceCreateForStore -v`
Expected: FAIL — `CreateStoreCouponInput` has no field `CouponType`/`DishIDs`/etc.

- [ ] **Step 3: Implement**

Add `"encoding/json"` and `"fmt"` to the import block if not already present (`strings` and `time` are already imported; `fmt` is not — add it).

Extend `CreateStoreCouponInput`:

```go
type CreateStoreCouponInput struct {
	Title              string
	Description        string
	Type               string
	CouponType         string
	Price              float64
	OriginalPrice      float64
	SalePrice          float64
	DiscountPercentage float64
	ImageURL           string
	DishIDs            []int64
	TotalQuantity      int
	MaxPerUser         int
	ValidFrom          *time.Time
	ValidUntil         *time.Time
	Terms              string
	Status             string
}
```

In `CreateForStore`, after the existing store/merchant validation and before `coupon := model.Coupon{...}`, resolve the image fallback and encode dish IDs:

```go
	imageURL := strings.TrimSpace(input.ImageURL)
	if imageURL == "" && len(input.DishIDs) == 1 {
		var dish model.Dish
		if err := s.db.WithContext(ctx).Where("id = ? AND merchant_id = ?", input.DishIDs[0], merchant.ID).First(&dish).Error; err == nil {
			imageURL = dish.ImageURL
		}
	}
	dishIDsJSON, err := json.Marshal(input.DishIDs)
	if err != nil {
		return nil, err
	}
```

Then add the new fields to the `coupon := model.Coupon{...}` literal:

```go
	coupon := model.Coupon{
		MerchantID:         store.MerchantID,
		StoreID:            &store.ID,
		Title:              title,
		Description:        input.Description,
		Type:               couponType,
		CouponType:         input.CouponType,
		Price:              input.Price,
		OriginalPrice:      input.OriginalPrice,
		SalePrice:          input.SalePrice,
		DiscountPercentage: input.DiscountPercentage,
		ImageURL:           imageURL,
		DishIDs:            string(dishIDsJSON),
		TotalQuantity:      input.TotalQuantity,
		MaxPerUser:         input.MaxPerUser,
		Terms:              input.Terms,
		Status:             couponStatusActive,
	}
```

(leave the rest of the existing field assignments — `Status` override, `ValidFrom`/`ValidUntil`/`ExpiryDate` — exactly as they are below this literal).

Note: `input.DishIDs` defaults to `nil` when unset, and `json.Marshal(nil)` for a `[]int64` produces `"null"`, not `"[]"`. Guard against that so the stored default matches the model's `default:'[]'` convention:

```go
	dishIDs := input.DishIDs
	if dishIDs == nil {
		dishIDs = []int64{}
	}
	dishIDsJSON, err := json.Marshal(dishIDs)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/core && go test ./internal/domain/coupon/... -v`
Expected: PASS for the two new tests and every pre-existing coupon test.

- [ ] **Step 5: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/domain/coupon/service/service.go apps/core/internal/domain/coupon/service/service_test.go
git commit -m "feat(coupon): populate pricing, dish association, and image fallback on create (#226)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 7: `computeStatus` + `ListForMerchant`

**Files:**
- Modify: `apps/core/internal/domain/coupon/service/service.go`
- Test: `apps/core/internal/domain/coupon/service/service_test.go`

**Interfaces:**
- Produces: `ComputeStatus(coupon model.Coupon, now time.Time) string` (exported — the handler package calls it in Task 9; `now` param makes it deterministically testable); `(*CouponService) ListForMerchant(ctx, userID, storeID int64) ([]model.Coupon, error)` — same ownership rules as `DeleteForStore`, but includes all statuses (soft-deleted excluded, same as any normal GORM query).

- [ ] **Step 1: Write the failing tests**

```go
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
```

Add `"time"` to the test file's imports (it likely already has it from the existing `ValidFrom`/`ValidUntil` usage — check first with `grep -n '"time"' apps/core/internal/domain/coupon/service/service_test.go` and add only if missing).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/core && go test ./internal/domain/coupon/service/... -run "TestComputeStatus|TestCouponServiceListForMerchant" -v`
Expected: FAIL — `computeStatus` and `ListForMerchant` undefined.

- [ ] **Step 3: Implement**

```go
const (
	couponStatusSoldOut   = "sold_out"
	couponStatusExpired   = "expired"
	couponStatusScheduled = "scheduled"
)

// ComputeStatus derives the effective, display-facing status for a coupon.
// draft/disabled are terminal, merchant-controlled states and are never
// overridden by quantity or date checks. now is passed in explicitly so
// this stays a pure, deterministically testable function. Exported because
// the handler package uses it to compute the status shown in API responses
// without persisting it (see Task 9).
func ComputeStatus(coupon model.Coupon, now time.Time) string {
	if coupon.Status == "draft" || coupon.Status == "disabled" {
		return coupon.Status
	}
	if coupon.TotalQuantity > 0 && coupon.ClaimedCount >= coupon.TotalQuantity {
		return couponStatusSoldOut
	}
	if coupon.ValidUntil != nil && coupon.ValidUntil.Before(now) {
		return couponStatusExpired
	}
	if coupon.ValidFrom != nil && coupon.ValidFrom.After(now) {
		return couponStatusScheduled
	}
	return coupon.Status
}

func (s *CouponService) ListForMerchant(ctx context.Context, userID, storeID int64) ([]model.Coupon, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreForbidden
		}
		return nil, err
	}

	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}
	if store.MerchantID != merchant.ID {
		return nil, ErrStoreForbidden
	}

	var coupons []model.Coupon
	if err := s.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Order("id desc").
		Find(&coupons).Error; err != nil {
		return nil, err
	}
	return coupons, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/core && go test ./internal/domain/coupon/... -v`
Expected: PASS for all new and existing coupon tests.

- [ ] **Step 5: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/domain/coupon/service/service.go apps/core/internal/domain/coupon/service/service_test.go
git commit -m "feat(coupon): add computeStatus and owner-scoped ListForMerchant (#226, #227)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 8: `UpdateForStore` + `SetEnabled`

**Files:**
- Modify: `apps/core/internal/domain/coupon/service/service.go`
- Test: `apps/core/internal/domain/coupon/service/service_test.go`

**Interfaces:**
- Produces: `type UpdateStoreCouponInput struct { Title, Description, CouponType, ImageURL *string; OriginalPrice, SalePrice, DiscountPercentage *float64; DishIDs *[]int64; TotalQuantity, MaxPerUser *int; ValidFrom, ValidUntil **time.Time; Terms, Status *string }`; `(*CouponService) UpdateForStore(ctx, userID, storeID, couponID int64, input UpdateStoreCouponInput) (*model.Coupon, error)`; `(*CouponService) SetEnabled(ctx, userID, storeID, couponID int64, enabled bool) (*model.Coupon, error)`.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/core && go test ./internal/domain/coupon/service/... -run "TestCouponServiceUpdateForStore|TestCouponServiceSetEnabled" -v`
Expected: FAIL — `UpdateStoreCouponInput`, `UpdateForStore`, `SetEnabled` undefined.

- [ ] **Step 3: Implement**

Add a shared ownership resolver to avoid repeating the merchant→store lookup a fourth time (refactor, but keep `DeleteForStore` and `ListForMerchant` as-is per YAGNI — only new methods use it, to minimize the diff on already-passing code):

```go
type UpdateStoreCouponInput struct {
	Title              *string
	Description        *string
	CouponType         *string
	ImageURL           *string
	OriginalPrice      *float64
	SalePrice          *float64
	DiscountPercentage *float64
	DishIDs            *[]int64
	TotalQuantity      *int
	MaxPerUser         *int
	ValidFrom          **time.Time
	ValidUntil         **time.Time
	Terms              *string
	Status             *string
}

func (s *CouponService) loadOwnedCoupon(ctx context.Context, userID, storeID, couponID int64) (*model.Merchant, *model.Coupon, error) {
	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrStoreForbidden
		}
		return nil, nil, err
	}

	var store model.Store
	if err := s.db.WithContext(ctx).First(&store, storeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrStoreNotFound
		}
		return nil, nil, err
	}
	if store.MerchantID != merchant.ID {
		return nil, nil, ErrStoreForbidden
	}

	var coupon model.Coupon
	if err := s.db.WithContext(ctx).First(&coupon, couponID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrCouponNotFound
		}
		return nil, nil, err
	}
	if coupon.StoreID == nil || *coupon.StoreID != storeID || coupon.MerchantID != merchant.ID {
		return nil, nil, ErrCouponNotFound
	}
	return &merchant, &coupon, nil
}

func (s *CouponService) UpdateForStore(ctx context.Context, userID, storeID, couponID int64, input UpdateStoreCouponInput) (*model.Coupon, error) {
	_, coupon, err := s.loadOwnedCoupon(ctx, userID, storeID, couponID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		if trimmed == "" {
			return nil, ErrInvalidCouponInput
		}
		updates["title"] = trimmed
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.CouponType != nil {
		updates["coupon_type"] = *input.CouponType
	}
	if input.ImageURL != nil {
		updates["image_url"] = *input.ImageURL
	}
	if input.OriginalPrice != nil {
		updates["original_price"] = *input.OriginalPrice
	}
	if input.SalePrice != nil {
		updates["sale_price"] = *input.SalePrice
	}
	if input.DiscountPercentage != nil {
		updates["discount_percentage"] = *input.DiscountPercentage
	}
	if input.DishIDs != nil {
		dishIDsJSON, err := json.Marshal(*input.DishIDs)
		if err != nil {
			return nil, err
		}
		updates["dish_ids"] = string(dishIDsJSON)
	}
	if input.TotalQuantity != nil {
		if *input.TotalQuantity < 0 {
			return nil, ErrInvalidCouponInput
		}
		updates["total_quantity"] = *input.TotalQuantity
	}
	if input.MaxPerUser != nil {
		if *input.MaxPerUser <= 0 {
			return nil, ErrInvalidCouponInput
		}
		updates["max_per_user"] = *input.MaxPerUser
	}
	if input.ValidFrom != nil {
		updates["valid_from"] = *input.ValidFrom
	}
	if input.ValidUntil != nil {
		updates["valid_until"] = *input.ValidUntil
		if *input.ValidUntil != nil {
			updates["expiry_date"] = **input.ValidUntil
		}
	}
	if input.Terms != nil {
		updates["terms"] = *input.Terms
	}
	if input.Status != nil {
		updates["status"] = *input.Status
	}
	if len(updates) == 0 {
		return coupon, nil
	}
	if err := s.db.WithContext(ctx).Model(&model.Coupon{}).Where("id = ?", couponID).Updates(updates).Error; err != nil {
		return nil, err
	}
	_, refreshed, err := s.loadOwnedCoupon(ctx, userID, storeID, couponID)
	return refreshed, err
}

func (s *CouponService) SetEnabled(ctx context.Context, userID, storeID, couponID int64, enabled bool) (*model.Coupon, error) {
	status := "disabled"
	if enabled {
		status = couponStatusActive
	}
	return s.UpdateForStore(ctx, userID, storeID, couponID, UpdateStoreCouponInput{Status: &status})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/core && go test ./internal/domain/coupon/... -v`
Expected: PASS for all new and existing coupon tests.

- [ ] **Step 5: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/domain/coupon/service/service.go apps/core/internal/domain/coupon/service/service_test.go
git commit -m "feat(coupon): add UpdateForStore and SetEnabled (#227)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 9: Coupon handler + routes for list-mine/update/enable/disable, and extend create DTO

**Files:**
- Modify: `apps/core/internal/domain/coupon/handler/coupon.go`
- Modify: `apps/core/internal/domain/stores/routes.go`

**Interfaces:**
- Consumes: `service.ListForMerchant`, `service.UpdateForStore`, `service.SetEnabled` (Tasks 7–8), extended `service.CreateStoreCouponInput` (Task 6).
- Produces: `GET /merchant/stores/:id/coupons`, `PATCH /merchant/stores/:id/coupons/:couponId`, `POST /merchant/stores/:id/coupons/:couponId/enable`, `POST /merchant/stores/:id/coupons/:couponId/disable`. Extends `CreateStoreCouponRequest` with the same new fields as `CreateStoreCouponInput`.

This task has no new Go unit tests of its own (this codebase's convention, confirmed in Task-adjacent exploration, is to test the `coupon` domain at the service layer only — `coupon/handler/coupon.go` has no existing `_test.go` file). Verification is the manual smoke test in Task 18's follow-up, plus the build/vet check below.

- [ ] **Step 0: Add a computed-status response wrapper**

The stored `model.Coupon.Status` column only holds `draft`/`active`/`disabled` — `sold_out`/`expired`/`scheduled` are derived by `service.ComputeStatus` (Task 7) and must never be written back to the database. Add a small wrapper so merchant-facing responses show the derived status without mutating the persisted row. Add near the top of `apps/core/internal/domain/coupon/handler/coupon.go`, below the existing type declarations:

```go
// couponResponse overrides model.Coupon's JSON "status" with the derived,
// display-facing status (see service.ComputeStatus) without persisting it.
// Go's json encoding prefers a shallower field over one promoted from an
// embedded struct, so this Status field wins over Coupon.Status at encode time.
type couponResponse struct {
	model.Coupon
	Status string `json:"status"`
}

func withComputedStatus(coupon model.Coupon) couponResponse {
	return couponResponse{Coupon: coupon, Status: service.ComputeStatus(coupon, time.Now())}
}

func withComputedStatuses(coupons []model.Coupon) []couponResponse {
	out := make([]couponResponse, len(coupons))
	for i, coupon := range coupons {
		out[i] = withComputedStatus(coupon)
	}
	return out
}
```

Add `"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"` to this file's imports (it isn't currently imported directly — the handler only used `service.*` types before).

- [ ] **Step 1: Extend `CreateStoreCouponRequest`**

In `apps/core/internal/domain/coupon/handler/coupon.go`:

```go
type CreateStoreCouponRequest struct {
	Title              string     `json:"title" binding:"required"`
	Description        string     `json:"description"`
	Type               string     `json:"type" binding:"required"`
	CouponType         string     `json:"coupon_type"`
	Price              float64    `json:"price"`
	OriginalPrice      float64    `json:"original_price"`
	SalePrice          float64    `json:"sale_price"`
	DiscountPercentage float64    `json:"discount_percentage"`
	ImageURL           string     `json:"image_url"`
	DishIDs            []int64    `json:"dish_ids"`
	TotalQuantity      int        `json:"total_quantity" binding:"required"`
	MaxPerUser         int        `json:"max_per_user" binding:"required"`
	ValidFrom          *time.Time `json:"valid_from"`
	ValidUntil         *time.Time `json:"valid_until"`
	Terms              string     `json:"terms"`
	Status             string     `json:"status"`
}

// UpdateStoreCouponRequest is the request payload for editing an existing
// store-scoped coupon. Every field is optional — only provided fields change.
type UpdateStoreCouponRequest struct {
	Title              *string    `json:"title"`
	Description        *string    `json:"description"`
	CouponType         *string    `json:"coupon_type"`
	ImageURL           *string    `json:"image_url"`
	OriginalPrice      *float64   `json:"original_price"`
	SalePrice          *float64   `json:"sale_price"`
	DiscountPercentage *float64   `json:"discount_percentage"`
	DishIDs            *[]int64   `json:"dish_ids"`
	TotalQuantity      *int       `json:"total_quantity"`
	MaxPerUser         *int       `json:"max_per_user"`
	ValidFrom          *time.Time `json:"valid_from"`
	ValidUntil         *time.Time `json:"valid_until"`
	Terms              *string    `json:"terms"`
	Status             *string    `json:"status"`
}
```

And update the `CreateForStore` call inside `CreateStoreCoupon` to pass the new fields through:

```go
	coupon, err := h.svc.CreateForStore(c.Request.Context(), userID, storeID, service.CreateStoreCouponInput{
		Title:              req.Title,
		Description:        req.Description,
		Type:               req.Type,
		CouponType:         req.CouponType,
		Price:              req.Price,
		OriginalPrice:      req.OriginalPrice,
		SalePrice:          req.SalePrice,
		DiscountPercentage: req.DiscountPercentage,
		ImageURL:           req.ImageURL,
		DishIDs:            req.DishIDs,
		TotalQuantity:      req.TotalQuantity,
		MaxPerUser:         req.MaxPerUser,
		ValidFrom:          req.ValidFrom,
		ValidUntil:         req.ValidUntil,
		Terms:              req.Terms,
		Status:             req.Status,
	})
```

Also update that same function's existing response line from `c.JSON(http.StatusCreated, gin.H{"data": coupon})` to `c.JSON(http.StatusCreated, gin.H{"data": withComputedStatus(*coupon)})` (defined in Step 0 below), so a freshly-created coupon's response already reflects its real derived status instead of only the raw stored value.

- [ ] **Step 2: Add the new handler methods**

Add below `ListStoreCoupons`:

```go
// ListMineForStore godoc
// @Summary List my store coupons
// @Description Lists all coupons for an owned store, including drafts/disabled/expired
// @Tags coupon
// @Produce json
// @Param id path int true "Store ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id}/coupons [get]
func (h *CouponHandler) ListMineForStore(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	storeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}
	coupons, err := h.svc.ListForMerchant(c.Request.Context(), userID, storeID)
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": withComputedStatuses(coupons)})
}

func parseStoreAndCouponID(c *gin.Context) (int64, int64, bool) {
	storeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return 0, 0, false
	}
	couponID, err := strconv.ParseInt(c.Param("couponId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid coupon id"})
		return 0, 0, false
	}
	return storeID, couponID, true
}

// UpdateStoreCoupon godoc
// @Summary Update store coupon
// @Tags coupon
// @Accept json
// @Produce json
// @Param id path int true "Store ID"
// @Param couponId path int true "Coupon ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id}/coupons/{couponId} [patch]
func (h *CouponHandler) UpdateStoreCoupon(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	storeID, couponID, ok := parseStoreAndCouponID(c)
	if !ok {
		return
	}
	var req UpdateStoreCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var validFrom, validUntil **time.Time
	if req.ValidFrom != nil {
		v := req.ValidFrom
		validFrom = &v
	}
	if req.ValidUntil != nil {
		v := req.ValidUntil
		validUntil = &v
	}
	coupon, err := h.svc.UpdateForStore(c.Request.Context(), userID, storeID, couponID, service.UpdateStoreCouponInput{
		Title: req.Title, Description: req.Description, CouponType: req.CouponType, ImageURL: req.ImageURL,
		OriginalPrice: req.OriginalPrice, SalePrice: req.SalePrice, DiscountPercentage: req.DiscountPercentage,
		DishIDs: req.DishIDs, TotalQuantity: req.TotalQuantity, MaxPerUser: req.MaxPerUser,
		ValidFrom: validFrom, ValidUntil: validUntil, Terms: req.Terms, Status: req.Status,
	})
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": withComputedStatus(*coupon)})
}

func (h *CouponHandler) setStoreCouponEnabled(c *gin.Context, enabled bool) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	storeID, couponID, ok := parseStoreAndCouponID(c)
	if !ok {
		return
	}
	coupon, err := h.svc.SetEnabled(c.Request.Context(), userID, storeID, couponID, enabled)
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": withComputedStatus(*coupon)})
}

// EnableStoreCoupon godoc
// @Summary Enable store coupon
// @Tags coupon
// @Produce json
// @Security BearerAuth
// @Router /merchant/stores/{id}/coupons/{couponId}/enable [post]
func (h *CouponHandler) EnableStoreCoupon(c *gin.Context) { h.setStoreCouponEnabled(c, true) }

// DisableStoreCoupon godoc
// @Summary Disable store coupon
// @Tags coupon
// @Produce json
// @Security BearerAuth
// @Router /merchant/stores/{id}/coupons/{couponId}/disable [post]
func (h *CouponHandler) DisableStoreCoupon(c *gin.Context) { h.setStoreCouponEnabled(c, false) }
```

Add `"time"` to the handler file's imports (it isn't currently imported there — `ValidFrom *time.Time` already exists in the pre-existing `CreateStoreCouponRequest`, so check first with `grep -n '"time"' apps/core/internal/domain/coupon/handler/coupon.go`; it's almost certainly already there since that field predates this task).

- [ ] **Step 3: Register the new routes**

In `apps/core/internal/domain/stores/routes.go`, add to the existing `merchantStoresAuth` group:

```go
	merchantStoresAuth := r.Group("/merchant/stores", authorization.JWTAuth(cfg.JWT))
	{
		merchantStoresAuth.GET("", storeH.ListMine)
		merchantStoresAuth.POST("", storeH.Create)
		merchantStoresAuth.POST("/:id/activate", storeH.Activate)
		merchantStoresAuth.POST("/:id/deactivate", storeH.Deactivate)
		merchantStoresAuth.PATCH("/:id", storeH.Update)
		merchantStoresAuth.DELETE("/:id", storeH.Delete)
		merchantStoresAuth.POST("/:id/coupons", couponH.CreateStoreCoupon)
		merchantStoresAuth.GET("/:id/coupons", couponH.ListMineForStore)
		merchantStoresAuth.PATCH("/:id/coupons/:couponId", couponH.UpdateStoreCoupon)
		merchantStoresAuth.POST("/:id/coupons/:couponId/enable", couponH.EnableStoreCoupon)
		merchantStoresAuth.POST("/:id/coupons/:couponId/disable", couponH.DisableStoreCoupon)
		merchantStoresAuth.DELETE("/:id/coupons/:couponId", couponH.DeleteStoreCoupon)
	}
```

- [ ] **Step 4: Verify the whole backend builds and tests pass**

Run: `cd apps/core && go build ./... && go vet ./... && go test ./...`
Expected: clean build, clean vet, all tests pass.

- [ ] **Step 5: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-core-api-go
git add apps/core/internal/domain/coupon/handler/coupon.go apps/core/internal/domain/stores/routes.go
git commit -m "feat(coupon): expose list-mine/update/enable/disable endpoints (#227)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

**This is the last backend task.** Before moving to the frontend, deploy/restart the backend against `revieu-dev` (or at minimum confirm `go build ./... && go test ./...` is green) so the frontend tasks below have real endpoints to call.

---

## Task 10: `storeProfileService` — create/activate/deactivate

**Files:**
- Modify: `revieu-web/src/features/merchant/profile/services/storeProfileService.ts`

**Interfaces:**
- Consumes: existing `apiClient` from `src/api/apiClient.ts`.
- Produces: `storeProfileService.createStore(payload: UpdateMerchantStorePayload): Promise<MerchantStoreRecord>`, `storeProfileService.activateStore(storeId: string): Promise<void>`, `storeProfileService.deactivateStore(storeId: string): Promise<void>`.

No test file is added here (per the spec's frontend-testing scope cut) — verify manually via the app in Task 11.

- [ ] **Step 1: Add the three methods**

In `revieu-web/src/features/merchant/profile/services/storeProfileService.ts`, add to the `storeProfileService` object (after `getPrimaryStore`, before `updateStore` — order doesn't matter functionally, but keep create next to get for readability):

```ts
export const storeProfileService = {
  async getPrimaryStore(): Promise<MerchantStoreRecord | null> {
    const response = await apiClient.get<{ data: MerchantStoreRecord[] }>('/merchant/stores?limit=1');
    return response.data.data[0] ?? null;
  },

  async createStore(payload: UpdateMerchantStorePayload): Promise<MerchantStoreRecord> {
    const response = await apiClient.post<{ data: MerchantStoreRecord }>('/merchant/stores', payload);
    return response.data.data;
  },

  async activateStore(storeId: string): Promise<void> {
    await apiClient.post(`/merchant/stores/${storeId}/activate`);
  },

  async deactivateStore(storeId: string): Promise<void> {
    await apiClient.post(`/merchant/stores/${storeId}/deactivate`);
  },

  async updateStore(
    storeId: string,
    payload: UpdateMerchantStorePayload
  ): Promise<MerchantStoreRecord> {
    const response = await apiClient.patch<{ data: MerchantStoreRecord }>(
      `/merchant/stores/${storeId}`,
      payload
    );

    return response.data.data;
  },
};
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/paul2/workspace/repos/revieu-web && npx tsc --noEmit`
Expected: no new type errors.

- [ ] **Step 3: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/profile/services/storeProfileService.ts
git commit -m "feat(store): add createStore/activateStore/deactivateStore to storeProfileService (#224)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 11: `StoreProfile.tsx` — create-store form branch

**Files:**
- Modify: `revieu-web/src/features/merchant/profile/pages/StoreProfile.tsx`

**Interfaces:**
- Consumes: `storeProfileService.createStore` (Task 10), existing `uploadMerchantImages`.

- [ ] **Step 1: Add creation state and a minimal create form, rendered instead of the dead-end message**

Replace the `if (!primaryStore) { ... }` block inside `loadStore` (around line 179) — keep setting `defaultStoreData` but drop the `setStatusMessage('No merchant store found yet.')` call, since the new branch below replaces that messaging with an actual form:

```ts
        if (!primaryStore) {
          setStoreData(defaultStoreData);
          setSavedStoreData(defaultStoreData);
          setIsCreatingStore(true);
          return;
        }
```

Add the new state near the other `useState` declarations at the top of the component (next to `isLoadingStore`):

```ts
  const [isCreatingStore, setIsCreatingStore] = useState(false);
  const [isSubmittingNewStore, setIsSubmittingNewStore] = useState(false);
```

Add a create handler near `handleSave`:

```ts
  const handleCreateStore = async () => {
    if (!storeData.name.trim()) {
      setStatusMessage('Store name is required.');
      return;
    }
    setIsSubmittingNewStore(true);
    setStatusMessage(null);
    try {
      const created = await storeProfileService.createStore(buildStoreUpdatePayload(storeData));
      const normalizedStore = normalizeStoreData(created, storeData);
      setStoreData(normalizedStore);
      setSavedStoreData(normalizedStore);
      setIsCreatingStore(false);
      setIsEditing(false);
    } catch (error) {
      console.error('Failed to create store:', error);
      setStatusMessage('Failed to create store.');
    } finally {
      setIsSubmittingNewStore(false);
    }
  };
```

- [ ] **Step 2: Render the create form when `isCreatingStore` is true**

Find the top-level return's opening wrapper (the outermost `<div>` that currently always renders the edit/view UI) and gate it: when `isCreatingStore` is true, render a form instead. Insert this branch immediately after the `isLoadingStore` early-return (search for `if (isLoadingStore)` — if the component doesn't have one, add the new branch right at the start of the JSX return, before the existing markup):

```tsx
  if (isCreatingStore) {
    return (
      <div className="max-w-2xl mx-auto p-6 space-y-4">
        <h1 className="text-2xl font-bold text-gray-900">Create your store</h1>
        <p className="text-sm text-gray-500">You don't have a store yet — fill in the basics to get started. You can add hours, photos, and menu images after creating it.</p>
        {statusMessage && (
          <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3">{statusMessage}</div>
        )}
        <div className="space-y-3">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Store name</label>
            <input
              type="text"
              value={storeData.name}
              onChange={(e) => setStoreData({ ...storeData, name: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg"
              placeholder="e.g. Downtown Burger House"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
            <textarea
              value={storeData.bio}
              onChange={(e) => setStoreData({ ...storeData, bio: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg"
              rows={3}
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Address</label>
            <input
              type="text"
              value={storeData.streetAddress}
              onChange={(e) => setStoreData({ ...storeData, streetAddress: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Phone</label>
            <input
              type="text"
              value={storeData.phone}
              onChange={(e) => setStoreData({ ...storeData, phone: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg"
            />
          </div>
        </div>
        <button
          onClick={handleCreateStore}
          disabled={isSubmittingNewStore}
          className="w-full py-3 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50"
        >
          {isSubmittingNewStore ? 'Creating...' : 'Create store'}
        </button>
      </div>
    );
  }
```

- [ ] **Step 3: Manually verify**

Run the frontend dev server (`cd /home/paul2/workspace/repos/revieu-web && npm run dev`), log in as a merchant with no store, navigate to the store profile page, confirm the create form renders, fill it in, submit, and confirm it switches into the normal edit view showing the newly created store (check the Network tab for a `201` from `POST /merchant/stores`).

- [ ] **Step 4: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/profile/pages/StoreProfile.tsx
git commit -m "feat(store): add create-store form when merchant has no store yet (#224)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 12: `StoreProfile.tsx` — enable/disable toggle

**Files:**
- Modify: `revieu-web/src/features/merchant/profile/pages/StoreProfile.tsx`

**Interfaces:**
- Consumes: `storeProfileService.activateStore`/`deactivateStore` (Task 10).

- [ ] **Step 1: Add active/inactive state and a toggle handler**

Add state near the others:

```ts
  const [isStoreActive, setIsStoreActive] = useState(false);
  const [isTogglingActive, setIsTogglingActive] = useState(false);
```

Backend doesn't currently return the store's `status` field on `MerchantStoreRecord` (only business fields) — add it:

In `storeProfileService.ts`, add `status?: number | null;` to `MerchantStoreRecord`. Then in `StoreProfile.tsx`'s `loadStore`, after `setSavedStoreData(normalizedStore)`, add:

```ts
        setIsStoreActive(primaryStore.status === 1);
```

Add the toggle handler:

```ts
  const handleToggleActive = async () => {
    if (!storeData.id) return;
    setIsTogglingActive(true);
    try {
      if (isStoreActive) {
        await storeProfileService.deactivateStore(storeData.id);
        setIsStoreActive(false);
      } else {
        await storeProfileService.activateStore(storeData.id);
        setIsStoreActive(true);
      }
    } catch (error) {
      console.error('Failed to toggle store active state:', error);
      setStatusMessage('Failed to update store status.');
    } finally {
      setIsTogglingActive(false);
    }
  };
```

- [ ] **Step 2: Render the toggle button**

Add it near the existing Save/Cancel/Edit button row (find the buttons around line 500-521 referenced by `disabled={isSaving || isUploadingImages}`) — add a sibling button:

```tsx
        <button
          onClick={handleToggleActive}
          disabled={isTogglingActive || !storeData.id}
          className={`px-4 py-2 rounded-lg font-medium disabled:opacity-50 ${
            isStoreActive ? 'bg-red-50 text-red-700 hover:bg-red-100' : 'bg-green-50 text-green-700 hover:bg-green-100'
          }`}
        >
          {isTogglingActive ? 'Updating...' : isStoreActive ? 'Disable store' : 'Enable store'}
        </button>
```

- [ ] **Step 3: Manually verify**

With the dev server running, toggle the button and confirm the network call hits `/activate` or `/deactivate` and the label flips.

- [ ] **Step 4: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/profile/pages/StoreProfile.tsx src/features/merchant/profile/services/storeProfileService.ts
git commit -m "feat(store): add enable/disable toggle to store profile (#224)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 13: `dishService.ts`

**Files:**
- Create: `revieu-web/src/features/merchant/dishes/services/dishService.ts`

**Interfaces:**
- Consumes: `apiClient` (`src/api/apiClient.ts`), `mediaApi`/`uploadToR2` (`src/api/media.ts`).
- Produces: `Dish` type, `dishService.list()`, `.create(payload)`, `.update(id, payload)`, `.remove(id)`, `.setEnabled(id, enabled)`, `.uploadImage(file)`.

- [ ] **Step 1: Write the service**

```ts
import { apiClient } from '../../../../api/apiClient';
import { mediaApi, uploadToR2 } from '../../../../api/media';

export interface Dish {
  id: number;
  merchant_id: number;
  name: string;
  image_url: string;
  description: string;
  original_price: number;
  category: string;
  status: 'active' | 'disabled';
}

export interface UpsertDishPayload {
  name: string;
  image_url?: string;
  description?: string;
  original_price: number;
  category?: string;
}

export const dishService = {
  async list(): Promise<Dish[]> {
    const response = await apiClient.get<{ data: Dish[] }>('/merchant/dishes');
    return response.data.data;
  },

  async create(payload: UpsertDishPayload): Promise<Dish> {
    const response = await apiClient.post<{ data: Dish }>('/merchant/dishes', payload);
    return response.data.data;
  },

  async update(id: number, payload: Partial<UpsertDishPayload>): Promise<Dish> {
    const response = await apiClient.patch<{ data: Dish }>(`/merchant/dishes/${id}`, payload);
    return response.data.data;
  },

  async remove(id: number): Promise<void> {
    await apiClient.delete(`/merchant/dishes/${id}`);
  },

  async setEnabled(id: number, enabled: boolean): Promise<Dish> {
    const response = await apiClient.post<{ data: Dish }>(`/merchant/dishes/${id}/${enabled ? 'enable' : 'disable'}`);
    return response.data.data;
  },

  async uploadImage(file: File): Promise<string> {
    const uploadUrlsResponse = await mediaApi.getUploadUrls({
      files: [{ filename: file.name, contentType: file.type || 'application/octet-stream' }],
    });
    const upload = uploadUrlsResponse.uploads[0];
    await uploadToR2(upload.uploadUrl, file);
    return upload.fileUrl;
  },
};
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/paul2/workspace/repos/revieu-web && npx tsc --noEmit`
Expected: no new type errors.

- [ ] **Step 3: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/dishes/services/dishService.ts
git commit -m "feat(dish): add dishService API client (#225)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 14: Dish management page + route + dashboard entry point

**Files:**
- Create: `revieu-web/src/features/merchant/dishes/pages/DishManagementPage.tsx`
- Create: `revieu-web/src/features/merchant/dishes/index.ts`
- Modify: `revieu-web/src/features/merchant/index.ts`
- Modify: `revieu-web/src/routes/paths.ts`
- Modify: `revieu-web/src/app/App.tsx`
- Modify: `revieu-web/src/features/merchant/dashboard/pages/MerchantDashboard.tsx`

**Interfaces:**
- Consumes: `dishService` (Task 13).
- Produces: `PATHS.MERCHANT.DISHES` route rendering `DishManagementPage`, reachable from a new Dashboard card.

- [ ] **Step 1: Add the route path**

In `revieu-web/src/routes/paths.ts`, add to the `MERCHANT` object (next to `ANALYTICS`):

```ts
        ANALYTICS: '/merchant/analytics',
        DISHES: '/merchant/dishes',
```

- [ ] **Step 2: Write the page**

```tsx
import React, { useEffect, useState } from 'react';
import { Plus, Edit3, Trash2, ImageIcon } from 'lucide-react';
import ConfirmationDialog from '../../shared/components/ConfirmationDialog';
import { Dish, UpsertDishPayload, dishService } from '../services/dishService';

const emptyForm: UpsertDishPayload = { name: '', description: '', original_price: 0, category: '', image_url: '' };

const DishManagementPage: React.FC = () => {
  const [dishes, setDishes] = useState<Dish[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [editingDishId, setEditingDishId] = useState<number | null>(null);
  const [form, setForm] = useState<UpsertDishPayload>(emptyForm);
  const [isSaving, setIsSaving] = useState(false);
  const [isUploadingImage, setIsUploadingImage] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<{ isOpen: boolean; dishId: number | null }>({ isOpen: false, dishId: null });

  const loadDishes = async () => {
    setIsLoading(true);
    try {
      setDishes(await dishService.list());
    } catch (error) {
      console.error('Failed to load dishes:', error);
      setStatusMessage('Failed to load dishes.');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadDishes();
  }, []);

  const openCreateModal = () => {
    setEditingDishId(null);
    setForm(emptyForm);
    setIsModalOpen(true);
  };

  const openEditModal = (dish: Dish) => {
    setEditingDishId(dish.id);
    setForm({ name: dish.name, description: dish.description, original_price: dish.original_price, category: dish.category, image_url: dish.image_url });
    setIsModalOpen(true);
  };

  const handleImageChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    setIsUploadingImage(true);
    try {
      const url = await dishService.uploadImage(file);
      setForm((prev) => ({ ...prev, image_url: url }));
    } catch (error) {
      console.error('Failed to upload dish image:', error);
      setStatusMessage('Failed to upload image.');
    } finally {
      setIsUploadingImage(false);
    }
  };

  const handleSubmit = async () => {
    if (!form.name.trim()) {
      setStatusMessage('Dish name is required.');
      return;
    }
    setIsSaving(true);
    setStatusMessage(null);
    try {
      if (editingDishId) {
        await dishService.update(editingDishId, form);
      } else {
        await dishService.create(form);
      }
      setIsModalOpen(false);
      await loadDishes();
    } catch (error) {
      console.error('Failed to save dish:', error);
      setStatusMessage('Failed to save dish.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleToggleEnabled = async (dish: Dish) => {
    try {
      await dishService.setEnabled(dish.id, dish.status !== 'active');
      await loadDishes();
    } catch (error) {
      console.error('Failed to toggle dish status:', error);
      setStatusMessage('Failed to update dish status.');
    }
  };

  const handleDelete = async () => {
    if (confirmDelete.dishId == null) return;
    try {
      await dishService.remove(confirmDelete.dishId);
      setConfirmDelete({ isOpen: false, dishId: null });
      await loadDishes();
    } catch (error) {
      console.error('Failed to delete dish:', error);
      setStatusMessage('Failed to delete dish.');
    }
  };

  return (
    <div className="p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Menu / Dishes</h1>
        <button onClick={openCreateModal} className="flex items-center gap-2 px-4 py-2 text-white bg-blue-600 rounded-lg hover:bg-blue-700">
          <Plus size={16} /> Add Dish
        </button>
      </div>
      {statusMessage && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3">{statusMessage}</div>}
      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : dishes.length === 0 ? (
        <p className="text-gray-500">No dishes yet. Click "Add Dish" to create your first one.</p>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {dishes.map((dish) => (
            <div key={dish.id} className="bg-white border border-gray-200 rounded-xl p-4 flex gap-3">
              <div className="w-16 h-16 rounded-lg bg-gray-100 overflow-hidden flex items-center justify-center shrink-0">
                {dish.image_url ? <img src={dish.image_url} alt={dish.name} className="w-full h-full object-cover" /> : <ImageIcon className="text-gray-400" size={24} />}
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center justify-between">
                  <h3 className="font-medium text-gray-900 truncate">{dish.name}</h3>
                  <span className={`text-xs px-2 py-0.5 rounded-full ${dish.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                    {dish.status}
                  </span>
                </div>
                <p className="text-sm text-gray-500">${dish.original_price.toFixed(2)} · {dish.category || 'Uncategorized'}</p>
                <div className="flex gap-2 mt-2">
                  <button onClick={() => openEditModal(dish)} className="text-blue-600 text-sm flex items-center gap-1"><Edit3 size={14} /> Edit</button>
                  <button onClick={() => handleToggleEnabled(dish)} className="text-gray-600 text-sm">{dish.status === 'active' ? 'Disable' : 'Enable'}</button>
                  <button onClick={() => setConfirmDelete({ isOpen: true, dishId: dish.id })} className="text-red-600 text-sm flex items-center gap-1"><Trash2 size={14} /> Delete</button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {isModalOpen && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-xl p-6 w-full max-w-md space-y-3">
            <h2 className="text-lg font-semibold">{editingDishId ? 'Edit Dish' : 'Add Dish'}</h2>
            <input type="text" placeholder="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />
            <textarea placeholder="Description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className="w-full px-3 py-2 border border-gray-300 rounded-lg" rows={2} />
            <input type="number" step="0.01" placeholder="Original price" value={form.original_price} onChange={(e) => setForm({ ...form, original_price: parseFloat(e.target.value) || 0 })} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />
            <input type="text" placeholder="Category" value={form.category} onChange={(e) => setForm({ ...form, category: e.target.value })} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />
            <input type="file" accept="image/*" onChange={handleImageChange} disabled={isUploadingImage} />
            {form.image_url && <img src={form.image_url} alt="preview" className="w-20 h-20 object-cover rounded-lg" />}
            <div className="flex gap-2 pt-2">
              <button onClick={() => setIsModalOpen(false)} className="flex-1 py-2 border border-gray-300 rounded-lg">Cancel</button>
              <button onClick={handleSubmit} disabled={isSaving} className="flex-1 py-2 bg-blue-600 text-white rounded-lg disabled:opacity-50">{isSaving ? 'Saving...' : 'Save'}</button>
            </div>
          </div>
        </div>
      )}

      <ConfirmationDialog
        isOpen={confirmDelete.isOpen}
        title="Delete Dish"
        message="Are you sure you want to delete this dish? This action cannot be undone."
        onConfirm={handleDelete}
        onClose={() => setConfirmDelete({ isOpen: false, dishId: null })}
      />
    </div>
  );
};

export default DishManagementPage;
```

Note: check `ConfirmationDialog`'s actual prop names first (`grep -n "interface.*Props" -A 8 src/features/merchant/shared/components/ConfirmationDialog.tsx`) and adjust the prop names above (`onClose` vs `onCancel`, etc.) to match exactly — other call sites in this codebase (e.g. `MerchantDashboard.tsx`'s `confirmDialog` usage) show the real prop shape.

- [ ] **Step 3: Mount the route**

This codebase re-exports every merchant page through barrel files, not by importing page files directly in `App.tsx` — `StoreAnalytics`/`AdManager` come from `revieu-web/src/features/merchant/dashboard/index.ts`, `StoreProfile` from `revieu-web/src/features/merchant/profile/index.ts`, both re-exported again from `revieu-web/src/features/merchant/index.ts`, which is what `App.tsx` actually imports from. Follow the same chain for the new page:

Create `revieu-web/src/features/merchant/dishes/index.ts`:

```ts
export { default as DishManagementPage } from './pages/DishManagementPage';
```

In `revieu-web/src/features/merchant/index.ts`, add a line under the existing `// Features` re-exports (next to `export * from './marketing';`):

```ts
export * from './dishes';
```

In `revieu-web/src/app/App.tsx`, add `DishManagementPage` to the existing destructured import block from `'../features/merchant'` (the one starting `import { MerchantLayout, MerchantDashboard, ... } from '../features/merchant';`):

```tsx
import {
  MerchantLayout,
  MerchantDashboard,
  VerificationPage,
  PostCreation,
  StoreAnalytics,
  AdManager,
  StoreProfile,
  DishManagementPage,
  Messages,
  ChatDetail,
  SearchMessages,
  Notifications
} from '../features/merchant';
```

Then add a sibling `<Route>` next to the existing `<Route path={PATHS.MERCHANT.ANALYTICS} element={<StoreAnalytics />} />` line:

```tsx
<Route path={PATHS.MERCHANT.DISHES} element={<DishManagementPage />} />
```

- [ ] **Step 4: Add a Dashboard entry point**

In `MerchantDashboard.tsx`, add a new card right after the existing "Store Analytics" card (inside the same `grid grid-cols-1 md:grid-cols-2 gap-4` block, so it becomes a 3rd/4th grid cell):

```tsx
        {/* Dish Management Card - Clickable */}
        <div
          className="bg-white rounded-xl shadow-sm border border-gray-200 p-6 cursor-pointer hover:shadow-md transition-shadow"
          onClick={() => navigate(PATHS.MERCHANT.DISHES)}
        >
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-600">Menu / Dishes</p>
              <p className="text-xs text-gray-500 mt-1">Manage your dishes and their images</p>
            </div>
            <div className="p-3 bg-orange-100 rounded-full">
              <UtensilsCrossed className="w-6 h-6 text-orange-600" />
            </div>
          </div>
        </div>
```

Add `UtensilsCrossed` to the `lucide-react` import at the top of `MerchantDashboard.tsx`.

- [ ] **Step 5: Manually verify**

With the dev server running, click the new card from the dashboard, confirm it navigates to `/merchant/dishes`, and confirm you can create/edit/enable/disable/delete a dish end-to-end against the real backend from Task 4.

- [ ] **Step 6: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/dishes/pages/DishManagementPage.tsx src/features/merchant/dishes/index.ts src/features/merchant/index.ts src/routes/paths.ts src/app/App.tsx src/features/merchant/dashboard/pages/MerchantDashboard.tsx
git commit -m "feat(dish): add dish management page + dashboard entry point (#225)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 15: `couponService.ts`

**Files:**
- Create: `revieu-web/src/features/merchant/marketing/services/couponService.ts`

**Interfaces:**
- Consumes: `apiClient`.
- Produces: `Coupon` type (mirrors backend `model.Coupon` JSON shape), `couponService.list(storeId)`, `.create(storeId, payload)`, `.update(storeId, couponId, payload)`, `.setEnabled(storeId, couponId, enabled)`, `.remove(storeId, couponId)`.

- [ ] **Step 1: Write the service**

```ts
import { apiClient } from '../../../../api/apiClient';

export interface Coupon {
  id: number;
  merchant_id: number;
  store_id: number | null;
  title: string;
  description: string;
  image_url: string;
  type: string;
  coupon_type: string;
  original_price: number;
  sale_price: number;
  discount_percentage: number;
  dish_ids: string; // JSON-encoded number[] — parse with JSON.parse before use
  total_quantity: number;
  claimed_count: number;
  max_per_user: number;
  valid_from: string | null;
  valid_until: string | null;
  status: string;
}

export interface UpsertCouponPayload {
  title: string;
  description?: string;
  type: string;
  coupon_type?: 'normal' | 'limited_time';
  original_price?: number;
  sale_price?: number;
  discount_percentage?: number;
  image_url?: string;
  dish_ids?: number[];
  total_quantity: number;
  max_per_user: number;
  valid_from?: string | null;
  valid_until?: string | null;
  terms?: string;
  status?: 'draft' | 'active';
}

export const couponService = {
  async list(storeId: string): Promise<Coupon[]> {
    const response = await apiClient.get<{ data: Coupon[] }>(`/merchant/stores/${storeId}/coupons`);
    return response.data.data;
  },

  async create(storeId: string, payload: UpsertCouponPayload): Promise<Coupon> {
    const response = await apiClient.post<{ data: Coupon }>(`/merchant/stores/${storeId}/coupons`, payload);
    return response.data.data;
  },

  async update(storeId: string, couponId: number, payload: Partial<UpsertCouponPayload>): Promise<Coupon> {
    const response = await apiClient.patch<{ data: Coupon }>(`/merchant/stores/${storeId}/coupons/${couponId}`, payload);
    return response.data.data;
  },

  async setEnabled(storeId: string, couponId: number, enabled: boolean): Promise<Coupon> {
    const response = await apiClient.post<{ data: Coupon }>(`/merchant/stores/${storeId}/coupons/${couponId}/${enabled ? 'enable' : 'disable'}`);
    return response.data.data;
  },

  async remove(storeId: string, couponId: number): Promise<void> {
    await apiClient.delete(`/merchant/stores/${storeId}/coupons/${couponId}`);
  },
};
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/paul2/workspace/repos/revieu-web && npx tsc --noEmit`

- [ ] **Step 3: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/marketing/services/couponService.ts
git commit -m "feat(coupon): add couponService API client (#226, #227)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 16: Coupon creation/edit form component

**Files:**
- Create: `revieu-web/src/features/merchant/marketing/components/CouponFormModal.tsx`

**Interfaces:**
- Consumes: `Dish` (Task 13, for the dish picker), `Coupon`/`UpsertCouponPayload` (Task 15).
- Produces: `<CouponFormModal isOpen coupon? dishes onClose onSubmit(payload, publish) />` — a controlled modal, no direct API calls (keeps this component testable/reusable regardless of which page hosts it, matching this codebase's existing `CouponManager`/`PackageManager` pattern of being dumb, parent-driven components).

- [ ] **Step 1: Write the component**

```tsx
import React, { useEffect, useState } from 'react';
import { Dish } from '../../dishes/services/dishService';
import { Coupon, UpsertCouponPayload } from '../services/couponService';

interface CouponFormModalProps {
  isOpen: boolean;
  coupon: Coupon | null;
  dishes: Dish[];
  onClose: () => void;
  onSubmit: (payload: UpsertCouponPayload) => Promise<void>;
}

const toDatetimeLocal = (iso: string | null): string => (iso ? iso.slice(0, 16) : '');

const CouponFormModal: React.FC<CouponFormModalProps> = ({ isOpen, coupon, dishes, onClose, onSubmit }) => {
  const [title, setTitle] = useState('');
  const [couponType, setCouponType] = useState<'normal' | 'limited_time'>('normal');
  const [selectedDishIds, setSelectedDishIds] = useState<number[]>([]);
  const [originalPrice, setOriginalPrice] = useState(0);
  const [salePrice, setSalePrice] = useState(0);
  const [totalQuantity, setTotalQuantity] = useState(1);
  const [validFrom, setValidFrom] = useState('');
  const [validUntil, setValidUntil] = useState('');
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    if (!isOpen) return;
    if (coupon) {
      setTitle(coupon.title);
      setCouponType(coupon.coupon_type === 'limited_time' ? 'limited_time' : 'normal');
      setSelectedDishIds(coupon.dish_ids ? (JSON.parse(coupon.dish_ids) as number[]) : []);
      setOriginalPrice(coupon.original_price);
      setSalePrice(coupon.sale_price);
      setTotalQuantity(coupon.total_quantity);
      setValidFrom(toDatetimeLocal(coupon.valid_from));
      setValidUntil(toDatetimeLocal(coupon.valid_until));
    } else {
      setTitle('');
      setCouponType('normal');
      setSelectedDishIds([]);
      setOriginalPrice(0);
      setSalePrice(0);
      setTotalQuantity(1);
      setValidFrom('');
      setValidUntil('');
    }
  }, [isOpen, coupon]);

  if (!isOpen) return null;

  const toggleDish = (dishId: number) => {
    setSelectedDishIds((prev) => (prev.includes(dishId) ? prev.filter((id) => id !== dishId) : [...prev, dishId]));
  };

  const discountPercentage = originalPrice > 0 ? Math.round(((originalPrice - salePrice) / originalPrice) * 100) : 0;

  const buildPayload = (status: 'draft' | 'active'): UpsertCouponPayload => ({
    title,
    type: 'percentage',
    coupon_type: couponType,
    original_price: originalPrice,
    sale_price: salePrice,
    discount_percentage: discountPercentage,
    dish_ids: selectedDishIds,
    total_quantity: totalQuantity,
    max_per_user: 1,
    valid_from: couponType === 'limited_time' && validFrom ? new Date(validFrom).toISOString() : null,
    valid_until: couponType === 'limited_time' && validUntil ? new Date(validUntil).toISOString() : null,
    status,
  });

  const handleSubmit = async (status: 'draft' | 'active') => {
    if (!title.trim() || originalPrice <= 0 || salePrice < 0 || totalQuantity <= 0) return;
    setIsSaving(true);
    try {
      await onSubmit(buildPayload(status));
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-xl p-6 w-full max-w-lg space-y-3 max-h-[90vh] overflow-y-auto">
        <h2 className="text-lg font-semibold">{coupon ? 'Edit Coupon' : 'Create Coupon'}</h2>

        <input type="text" placeholder="Coupon name" value={title} onChange={(e) => setTitle(e.target.value)} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />

        <div className="flex gap-2">
          <button type="button" onClick={() => setCouponType('normal')} className={`flex-1 py-2 rounded-lg border ${couponType === 'normal' ? 'bg-blue-600 text-white border-blue-600' : 'border-gray-300'}`}>Normal Coupon</button>
          <button type="button" onClick={() => setCouponType('limited_time')} className={`flex-1 py-2 rounded-lg border ${couponType === 'limited_time' ? 'bg-blue-600 text-white border-blue-600' : 'border-gray-300'}`}>Limited-Time Coupon</button>
        </div>

        <div>
          <p className="text-sm font-medium text-gray-700 mb-1">Applicable dishes (none selected = all dishes)</p>
          <div className="flex flex-wrap gap-2 max-h-28 overflow-y-auto">
            {dishes.map((dish) => (
              <button
                key={dish.id}
                type="button"
                onClick={() => toggleDish(dish.id)}
                className={`px-3 py-1 rounded-full text-sm border ${selectedDishIds.includes(dish.id) ? 'bg-blue-100 border-blue-400 text-blue-800' : 'border-gray-300 text-gray-600'}`}
              >
                {dish.name}
              </button>
            ))}
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-sm text-gray-700 mb-1">Original price</label>
            <input type="number" step="0.01" value={originalPrice} onChange={(e) => setOriginalPrice(parseFloat(e.target.value) || 0)} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />
          </div>
          <div>
            <label className="block text-sm text-gray-700 mb-1">Discount price</label>
            <input type="number" step="0.01" value={salePrice} onChange={(e) => setSalePrice(parseFloat(e.target.value) || 0)} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />
          </div>
        </div>
        <p className="text-sm text-gray-500">${originalPrice.toFixed(2)} → ${salePrice.toFixed(2)} ({discountPercentage}% off)</p>

        <div>
          <label className="block text-sm text-gray-700 mb-1">Total quantity</label>
          <input type="number" value={totalQuantity} onChange={(e) => setTotalQuantity(parseInt(e.target.value, 10) || 0)} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />
        </div>

        {couponType === 'limited_time' && (
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-sm text-gray-700 mb-1">Start time</label>
              <input type="datetime-local" value={validFrom} onChange={(e) => setValidFrom(e.target.value)} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />
            </div>
            <div>
              <label className="block text-sm text-gray-700 mb-1">End time</label>
              <input type="datetime-local" value={validUntil} onChange={(e) => setValidUntil(e.target.value)} className="w-full px-3 py-2 border border-gray-300 rounded-lg" />
            </div>
          </div>
        )}

        <div className="flex gap-2 pt-2">
          <button onClick={onClose} className="flex-1 py-2 border border-gray-300 rounded-lg">Cancel</button>
          <button onClick={() => handleSubmit('draft')} disabled={isSaving} className="flex-1 py-2 border border-blue-600 text-blue-600 rounded-lg disabled:opacity-50">Save as Draft</button>
          <button onClick={() => handleSubmit('active')} disabled={isSaving} className="flex-1 py-2 bg-blue-600 text-white rounded-lg disabled:opacity-50">Publish</button>
        </div>
      </div>
    </div>
  );
};

export default CouponFormModal;
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/paul2/workspace/repos/revieu-web && npx tsc --noEmit`

- [ ] **Step 3: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/marketing/components/CouponFormModal.tsx
git commit -m "feat(coupon): add coupon create/edit form modal (#226)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 17: Horizontal coupon list component

**Files:**
- Create: `revieu-web/src/features/merchant/marketing/components/CouponHorizontalList.tsx`

**Interfaces:**
- Consumes: `Coupon`, `Dish` (Tasks 13, 15).
- Produces: `<CouponHorizontalList coupons dishes onEdit(coupon) onToggleEnabled(coupon) onDelete(coupon) />`.

- [ ] **Step 1: Write the component**

```tsx
import React from 'react';
import { Edit3, Trash2, ImageIcon } from 'lucide-react';
import { Coupon } from '../services/couponService';
import { Dish } from '../../dishes/services/dishService';

interface CouponHorizontalListProps {
  coupons: Coupon[];
  dishes: Dish[];
  onEdit: (coupon: Coupon) => void;
  onToggleEnabled: (coupon: Coupon) => void;
  onDelete: (coupon: Coupon) => void;
}

const statusStyles: Record<string, string> = {
  active: 'bg-green-100 text-green-700',
  draft: 'bg-gray-100 text-gray-600',
  disabled: 'bg-gray-100 text-gray-500',
  scheduled: 'bg-blue-100 text-blue-700',
  expired: 'bg-red-100 text-red-700',
  sold_out: 'bg-orange-100 text-orange-700',
};

const formatTimeRange = (validFrom: string | null, validUntil: string | null): string | null => {
  if (!validFrom && !validUntil) return null;
  const fmt = (iso: string) => new Date(iso).toLocaleString(undefined, { hour: 'numeric', minute: '2-digit' });
  if (validFrom && validUntil) return `${fmt(validFrom)} - ${fmt(validUntil)}`;
  return validFrom ? `From ${fmt(validFrom)}` : `Until ${fmt(validUntil as string)}`;
};

const CouponHorizontalList: React.FC<CouponHorizontalListProps> = ({ coupons, dishes, onEdit, onToggleEnabled, onDelete }) => {
  if (coupons.length === 0) {
    return <p className="text-gray-500 text-center py-4">No coupons yet. Click "Create Coupon" to make your first one.</p>;
  }

  return (
    <div className="space-y-3">
      {coupons.map((coupon) => {
        const dishIds: number[] = coupon.dish_ids ? JSON.parse(coupon.dish_ids) : [];
        const dishNames = dishIds.length === 0 ? 'All dishes' : dishIds.map((id) => dishes.find((d) => d.id === id)?.name).filter(Boolean).join(', ') || 'Selected dishes';
        const timeRange = formatTimeRange(coupon.valid_from, coupon.valid_until);

        return (
          <div key={coupon.id} className="flex items-center gap-4 border border-gray-200 rounded-lg p-3">
            <div className="w-16 h-16 rounded-lg bg-gray-100 overflow-hidden flex items-center justify-center shrink-0">
              {coupon.image_url ? <img src={coupon.image_url} alt={coupon.title} className="w-full h-full object-cover" /> : <ImageIcon className="text-gray-400" size={24} />}
            </div>

            <div className="flex-1 min-w-0">
              <p className="font-medium text-gray-900 truncate">{coupon.title}</p>
              <p className="text-xs text-gray-500">{coupon.coupon_type === 'limited_time' ? 'Limited-Time Coupon' : 'Normal Coupon'} · {dishNames}</p>
              <p className="text-sm text-gray-700">${coupon.original_price.toFixed(2)} → ${coupon.sale_price.toFixed(2)}</p>
            </div>

            <div className="text-right shrink-0">
              <p className="text-sm text-gray-700">{coupon.claimed_count} / {coupon.total_quantity} claimed</p>
              {timeRange && <p className="text-xs text-gray-500">{timeRange}</p>}
              <span className={`inline-block mt-1 text-xs px-2 py-0.5 rounded-full ${statusStyles[coupon.status] ?? 'bg-gray-100 text-gray-600'}`}>{coupon.status}</span>
            </div>

            <div className="flex flex-col gap-1 shrink-0">
              <button onClick={() => onEdit(coupon)} className="text-blue-600 text-sm flex items-center gap-1"><Edit3 size={14} /> Edit</button>
              <button onClick={() => onToggleEnabled(coupon)} className="text-gray-600 text-sm">{coupon.status === 'disabled' ? 'Enable' : 'Disable'}</button>
              <button onClick={() => onDelete(coupon)} className="text-red-600 text-sm flex items-center gap-1"><Trash2 size={14} /> Delete</button>
            </div>
          </div>
        );
      })}
    </div>
  );
};

export default CouponHorizontalList;
```

Note: the derived statuses (`scheduled`/`expired`/`sold_out`) shown in `coupon.status` above rely on Task 9's `withComputedStatus`/`withComputedStatuses` wrapper — `GET /merchant/stores/:id/coupons` and the update/enable/disable/create responses all serialize `service.ComputeStatus(...)`, not the raw stored `Status` column, so this component can trust `coupon.status` as-is.

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/paul2/workspace/repos/revieu-web && npx tsc --noEmit`

- [ ] **Step 3: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/marketing/components/CouponHorizontalList.tsx
git commit -m "feat(coupon): add horizontal coupon list component (#227)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Task 18: Wire real coupons into `MerchantDashboard.tsx` (fixes the local-only-state bug)

**Files:**
- Modify: `revieu-web/src/features/merchant/dashboard/pages/MerchantDashboard.tsx`

**Interfaces:**
- Consumes: `couponService` (Task 15), `dishService` (Task 13), `CouponFormModal` (Task 16), `CouponHorizontalList` (Task 17), `storeProfileService.getPrimaryStore` (existing).

- [ ] **Step 1: Replace imports**

Remove:
```ts
import CouponManager from '../../marketing/components/CouponManager';
```
Add:
```ts
import CouponFormModal from '../../marketing/components/CouponFormModal';
import CouponHorizontalList from '../../marketing/components/CouponHorizontalList';
import { Coupon, couponService } from '../../marketing/services/couponService';
import { Dish, dishService } from '../../dishes/services/dishService';
import { storeProfileService } from '../../profile/services/storeProfileService';
```

- [ ] **Step 2: Replace coupon state and add store/dish loading**

Replace `const [coupons, setCoupons] = useState<any[]>([]);` with:

```ts
  const [coupons, setCoupons] = useState<Coupon[]>([]);
  const [dishes, setDishes] = useState<Dish[]>([]);
  const [storeId, setStoreId] = useState<string | null>(null);
  const [isCouponFormOpen, setIsCouponFormOpen] = useState(false);
  const [editingCoupon, setEditingCoupon] = useState<Coupon | null>(null);
```

Add a load effect near the top of the component body (after the existing `useState` declarations, before the helper functions):

```ts
  useEffect(() => {
    const loadStoreAndCoupons = async () => {
      try {
        const store = await storeProfileService.getPrimaryStore();
        if (!store) return;
        setStoreId(String(store.id));
        const [fetchedCoupons, fetchedDishes] = await Promise.all([
          couponService.list(String(store.id)),
          dishService.list(),
        ]);
        setCoupons(fetchedCoupons);
        setDishes(fetchedDishes);
      } catch (error) {
        console.error('Failed to load coupons:', error);
      }
    };
    void loadStoreAndCoupons();
  }, []);
```

Add `useEffect` to the React import at the top of the file (currently `import React, { useState } from 'react';`):

```ts
import React, { useEffect, useState } from 'react';
```

- [ ] **Step 3: Remove the fake `handleUpdateCoupons` and add real create/edit/enable/delete handlers**

Delete:
```ts
  const handleUpdateCoupons = (updatedCoupons: any[]) => {
    setCoupons(updatedCoupons);
    // Force a re-render to ensure the dashboard reflects changes immediately
    console.log('Coupons updated:', updatedCoupons);
  };
```

Add in its place:

```ts
  const refreshCoupons = async () => {
    if (!storeId) return;
    setCoupons(await couponService.list(storeId));
  };

  const handleCreateOrUpdateCoupon = async (payload: Parameters<typeof couponService.create>[1]) => {
    if (!storeId) return;
    if (editingCoupon) {
      await couponService.update(storeId, editingCoupon.id, payload);
    } else {
      await couponService.create(storeId, payload);
    }
    setIsCouponFormOpen(false);
    setEditingCoupon(null);
    await refreshCoupons();
  };

  const handleToggleCouponEnabled = async (coupon: Coupon) => {
    if (!storeId) return;
    await couponService.setEnabled(storeId, coupon.id, coupon.status === 'disabled');
    await refreshCoupons();
  };

  const handleDeleteCoupon = (coupon: Coupon) => {
    if (!storeId) return;
    setConfirmDialog({
      isOpen: true,
      title: 'Delete Coupon',
      message: `Are you sure you want to delete "${coupon.title}"? This action cannot be undone.`,
      onConfirm: () => {
        void (async () => {
          await couponService.remove(storeId, coupon.id);
          await refreshCoupons();
          closeConfirmDialog();
        })();
      },
    });
  };
```

- [ ] **Step 4: Replace the "Active Coupons" JSX block and the `CouponManager` usage**

Replace the entire block from `{/* Active Coupons */}` through its closing `</div>` (the section using `coupons.filter(coupon => coupon.isActive)`, `expandedCoupons`, etc.) with:

```tsx
      {/* Coupons */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900">Coupons</h2>
          <button
            onClick={() => { setEditingCoupon(null); setIsCouponFormOpen(true); }}
            disabled={!storeId}
            className="flex items-center gap-2 px-4 py-2 text-white rounded-lg hover:bg-yellow-600 transition-colors disabled:opacity-50"
            style={{ backgroundColor: '#FFBC0D' }}
          >
            <Gift size={16} />
            Create Coupon
          </button>
        </div>
        <CouponHorizontalList
          coupons={coupons}
          dishes={dishes}
          onEdit={(coupon) => { setEditingCoupon(coupon); setIsCouponFormOpen(true); }}
          onToggleEnabled={handleToggleCouponEnabled}
          onDelete={handleDeleteCoupon}
        />
      </div>
```

This removes the now-unused `expandedCoupons`/`toggleCouponExpansion` state and handler too — since nothing else in the file references them after this change, delete their declarations (`const [expandedCoupons, setExpandedCoupons] = useState<Set<number>>(new Set());` and the `toggleCouponExpansion` function) to avoid an unused-variable lint failure. Confirm with `grep -n "expandedCoupons\|toggleCouponExpansion" src/features/merchant/dashboard/pages/MerchantDashboard.tsx` that no other usage remains before deleting.

Replace the `<CouponManager ... />` block (the one wired to `showCouponManager`/`handleUpdateCoupons`) with:

```tsx
      <CouponFormModal
        isOpen={isCouponFormOpen}
        coupon={editingCoupon}
        dishes={dishes}
        onClose={() => { setIsCouponFormOpen(false); setEditingCoupon(null); }}
        onSubmit={handleCreateOrUpdateCoupon}
      />
```

Also remove the now-unused `showCouponManager` state (`const [showCouponManager, setShowCouponManager] = useState(false);`) and its one remaining reference (the "Edit Coupons" button you just replaced) — confirm with `grep -n "showCouponManager" src/features/merchant/dashboard/pages/MerchantDashboard.tsx` that nothing else references it before deleting.

- [ ] **Step 5: Manually verify the bug is actually fixed**

With the dev server + backend running end-to-end (through the SSH tunnel to `revieu-dev` if testing against that environment, or a local backend):
1. Open the merchant dashboard, click "Create Coupon", fill in a Normal Coupon, publish it.
2. Confirm it appears in the horizontal list immediately.
3. **Reload the page.** Confirm the coupon is still there (this is the concrete regression test for the bug described in the spec — before this task, a reload would have lost it because it only lived in React state).
4. Disable it, confirm the status badge updates; delete it, confirm it disappears; reload again to confirm the delete persisted.

- [ ] **Step 6: Commit**

```bash
cd /home/paul2/workspace/repos/revieu-web
git add src/features/merchant/dashboard/pages/MerchantDashboard.tsx
git commit -m "fix(coupon): wire dashboard coupon UI to real backend, replacing local-only state

The dashboard's coupon create/edit/delete flow previously only updated
React state and never called the backend — coupons vanished on reload
and never reached the store's real coupon list. Replaces the CouponManager
modal with CouponFormModal + CouponHorizontalList backed by couponService.

Fixes #226, #227.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_016sLjSF1N1rdSSxTtXV43LD"
```

---

## Final check (all tasks complete)

- [ ] Backend: `cd apps/core && go build ./... && go vet ./... && go test ./...` — all green.
- [ ] Frontend: `cd /home/paul2/workspace/repos/revieu-web && npx tsc --noEmit` — no new errors.
- [ ] Manual end-to-end smoke test per the spec's Testing section: create a store (with `AUTO_VERIFY_NEW_MERCHANTS=true` set on the revieu-dev backend), confirm it's visible via the customer app, create a dish, create both a Normal and a Limited-Time coupon against it, reload, confirm everything persisted.
- [ ] `CouponManager.tsx` and its now-unused import in `MerchantDashboard.tsx` are fully removed from the dashboard's render path — leave the file itself in place (don't delete `marketing/components/CouponManager.tsx`) unless nothing else in the codebase still imports it (`grep -rln "CouponManager" revieu-web/src`); if it's now dead code, note that to the user rather than deleting it unprompted.
