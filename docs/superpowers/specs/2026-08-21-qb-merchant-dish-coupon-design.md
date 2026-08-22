# QB: Merchant, Dish & Coupon Management — Design

- **Issues:** revieu-core-api-go#223 (epic), #224 (Merchant), #225 (Dish/Menu), #226 (Coupon Creation), #227 (Coupon List/Management). #228 (Review Management) is explicitly out of scope for this pass.
- **Repos touched:** `revieu-core-api-go` (backend), `revieu-web` (frontend, existing `features/merchant` app — no new "QB" namespace is created; QB requirements are implemented by extending the existing B-side merchant app, since it already owns login/dashboard/store-profile plumbing).
- **Driver:** live demo. Depth target is "demo-correct end-to-end", not full production hardening (documented explicitly per section below).

## Context

Investigation of the existing codebase surfaced the following facts, which shape every decision below:

1. Backend already has working store CRUD (`POST/PATCH/DELETE /merchant/stores`, `activate`/`deactivate`) with real fields for name/logo/cover/description/address/phone/hours/images. Nothing here needs to change structurally.
2. The frontend `StoreProfile.tsx` only has an **edit** path. When a merchant has no store yet, the page shows "No merchant store found yet." and the save handler unconditionally fails with "Store context is missing." — there is no "create" branch at all. This is the actual gap behind issue #224, not a missing backend.
3. There is no enable/disable control in the store profile UI, even though the backend `activate`/`deactivate` endpoints exist.
4. There is no `Dish` concept anywhere in the codebase (frontend or backend). Issue #225 is greenfield.
5. `model.Coupon` already carries most of the fields issue #226 needs (`OriginalPrice`, `SalePrice`, `DiscountPercentage`, `TotalQuantity`, `ClaimedCount`, `ValidFrom`, `ValidUntil`, `Status`, `ImageURL`). It has no way to associate a coupon with specific dishes, and `CreateForStore` does not populate the pricing/image fields at all.
5b. `MerchantDashboard.tsx`'s coupon UI (`CouponManager`) is **entirely local React state** — `handleUpdateCoupons` calls `setCoupons(...)` and nothing else. It never calls the real `POST/DELETE /merchant/stores/:id/coupons` endpoints that already exist. This is a pre-existing bug, unrelated to #223, that must be fixed as part of this work since the demo depends on coupons actually persisting.
6. There is no merchant-facing "list my coupons including drafts/disabled/expired" endpoint (the public one only returns active coupons), and no edit or enable/disable endpoint for coupons.
7. Customer-facing merchant detail (`GET /merchants/:id`) is gated on `Merchant.status = 0 AND Merchant.verification_status = 'verified'`. A merchant created through the new QB flow would not satisfy this by default, meaning a freshly-created merchant would be invisible on the customer app during the demo.

## Scope decisions (confirmed with user)

- Build order/priority: #224 → #225 → #226 → #227. #228 (review moderation) is deliberately excluded from this pass.
- New merchants must appear on the customer-facing app immediately after creation for demo purposes. This is implemented behind a config flag (`AUTO_VERIFY_NEW_MERCHANTS`, default `false`), not as an unconditional change to verification behavior — bypassing merchant verification is a product/trust decision, not something this change makes permanent for every environment. The flag will be enabled only in `revieu-dev` for the demo.
- No new "QB" UI namespace/app is created. All QB requirements are implemented inside the existing `revieu-web/src/features/merchant` app, reusing its layout, auth, and API client.
- Dish records are merchant-private data in this pass (no public dish endpoint). A dish's `ImageURL` is copied onto a coupon's `ImageURL` at coupon-creation time; the customer-facing coupon browsing/claiming flow itself is out of scope (per the epic's own "Out of scope" list).
- Coupon status (`sold_out` / `expired` / `scheduled` / `active`) is computed on read, not stored via a background job — avoids stale-state bugs and a cron/worker for a one-day-turnaround feature. Only `disabled` (merchant toggle) is a stored, authoritative flag.

## Backend design (`revieu-core-api-go`)

### 1. Demo-only auto-verify flag

- `config.Config` gains `Features.AutoVerifyNewMerchants bool`, sourced the same way as other config (`config.Load()`), default `false`.
- `StoreService.Create` (and `Activate`), after a successful store create/activate, checks this flag; if true, updates the owning `Merchant` row to `status = 0, verification_status = "verified"`. This is a small, clearly-commented addition — not a change to the verification service/domain itself.

### 2. `dish` domain (new)

New package `internal/domain/dish`, following the existing `merchant`/`coupon` domain shape (`dto/`, `service/`, `handler/`, `routes.go`).

- `model.Dish`: `ID, MerchantID (indexed, not null), Name, ImageURL, Description, OriginalPrice float64, Category string, Status string ("active"|"disabled", default "active"), CreatedAt, UpdatedAt, DeletedAt (soft delete)`. Added to `model.All()`.
- Service methods: `Create`, `Update`, `Delete` (soft), `ListMine(ctx, userID)`, all scoped to the caller's `Merchant` (resolved via `user_id` → `Merchant.UserID`, same pattern as `CreateForStore`). `SetStatus(ctx, userID, dishID, status)` for enable/disable.
- Routes (all under existing `authorization.JWTAuth`):
  - `POST /merchant/dishes`
  - `GET /merchant/dishes`
  - `PATCH /merchant/dishes/:id`
  - `DELETE /merchant/dishes/:id`
  - `POST /merchant/dishes/:id/enable`, `POST /merchant/dishes/:id/disable`
- Image upload reuses the existing generic media/R2 presigned-upload flow already used for store images — no new backend upload path.

### 3. Coupon extensions (existing `coupon` domain)

- Model: add `DishIDs string` (`jsonb`, default `'[]'`) — array of dish IDs the coupon applies to; empty array means "all dishes" per the issue's requirement. Add `CouponType` is already present (`normal` / `limited_time` values, previously unused — now populated).
- `CreateStoreCouponInput` (service) and `CreateStoreCouponRequest` (handler DTO) gain: `OriginalPrice`, `SalePrice`, `DiscountPercentage`, `CouponType`, `DishIDs []int64`, `ImageURL` (optional — if empty and exactly one dish is referenced, service copies that dish's `ImageURL`).
- New service methods:
  - `ListForMerchant(ctx, userID, storeID)` — owner-scoped, all statuses (draft/disabled/expired/sold-out/active), unlike the existing public `ListPublishedByStore`.
  - `UpdateForStore(ctx, userID, storeID, couponID, input)` — edits the same field set as create.
  - `SetEnabled(ctx, userID, storeID, couponID, enabled bool)`.
  - `computeStatus(coupon) string` helper used when mapping to DTO: `sold_out` if `ClaimedCount >= TotalQuantity`, else `expired` if `ValidUntil` is set and in the past, else `scheduled` if `ValidFrom` is set and in the future, else the stored `Status` (`active`/`disabled`/`draft`).
  - Draft vs. publish: the create/edit form has an explicit "Save as Draft" vs. "Publish" choice, sent as `Status: "draft"` or `"active"` on the request (mirrors the existing `CreateStoreCouponRequest.Status` field, which already flows through unchanged). A draft is never surfaced by `computeStatus` as `scheduled`/`active` regardless of its dates — draft is a terminal merchant-controlled state until the merchant explicitly publishes it (`PATCH .../coupons/:couponId` with `Status: "active"`).
- New routes (under existing `merchantStoresAuth` group in `stores/routes.go`):
  - `GET /merchant/stores/:id/coupons`
  - `PATCH /merchant/stores/:id/coupons/:couponId`
  - `POST /merchant/stores/:id/coupons/:couponId/enable`, `POST .../disable`

## Frontend design (`revieu-web`, inside existing `features/merchant`)

### 1. Store creation flow (fixes #224's core gap)

- `StoreProfile.tsx`: when `getPrimaryStore()` resolves `null`, render a "Create your store" form (name/logo/cover/description/address/phone/category — hours can be filled in after creation, matching the existing edit form's own field set) instead of the current dead-end message. Submitting calls `storeProfileService.createStore(payload)` (new method, `POST /merchant/stores`), then transitions into the normal edit view with the returned store.
- Add an enable/disable toggle to the same page, calling new `storeProfileService.activateStore` / `deactivateStore` methods against the existing backend endpoints.

### 2. Dish management (new, #225)

- New `features/merchant/dishes/` slice: `services/dishService.ts` (CRUD against the new backend routes), `pages/DishManagementPage.tsx` (list + add/edit modal + enable/disable + image upload via the existing `uploadMerchantImages` helper), route `PATHS.MERCHANT.DISHES`.
- Entry point: a new card on `MerchantDashboard.tsx`, styled like the existing "Store Analytics" card (dashboard already uses this card-linking pattern for pages not on the bottom tab bar — avoids reworking the 5-slot bottom nav).

### 3. Coupon creation + management (#226, #227 — replaces the fake `CouponManager` flow)

- New `features/merchant/marketing/services/couponService.ts`: `list(storeId)`, `create(storeId, payload)`, `update(storeId, couponId, payload)`, `setEnabled(storeId, couponId, enabled)`, `remove(storeId, couponId)` — thin wrappers over the new/existing backend routes.
- `MerchantDashboard.tsx`: replace local-only `coupons` state with data fetched via `couponService.list(storeId)` on mount; `handleUpdateCoupons` is removed in favor of the granular create/update/delete calls above, each followed by a refetch (or optimistic local update) so the dashboard reflects the real backend state.
- New coupon creation form (type toggle Normal/Limited-Time, dish picker sourced from `dishService.list()`, original/discount price, quantity, start/end time shown only for Limited-Time).
- New horizontal coupon list component per the issue's mock (dish image left, name/type/dish/price center, quantity/time/status right, edit/enable-disable/delete actions), replacing the vertical `CouponManager` modal.

## Testing

- Backend: TDD per existing convention — `dish` domain gets handler/service tests mirroring `coupon`'s (`service_test.go` pattern: in-memory sqlite via `testutil`, ownership/forbidden/not-found cases). Coupon extensions get tests for `computeStatus` (all four derived states) and the new list/update/enable endpoints (ownership checks, same pattern as `TestCouponServiceDeleteForStoreForbiddenForNonOwner`).
- Frontend: given the timeline, no new test suite is written for the new pages; existing test patterns (`StoreProfile.test.tsx`) are left as reference but not required to gain coverage for the new create-branch in this pass. This is a deliberate scope cut, called out explicitly rather than silently skipped.
- Manual smoke test before the demo: create a store end-to-end through the new UI, confirm it appears via the customer app (with the flag on in revieu-dev), create a dish, create both coupon types against real backend endpoints, confirm the coupon list reflects true persisted state after a page reload.

## Explicitly out of scope for this pass

- Issue #228 (review moderation).
- Customer-side coupon browsing/claiming changes (already excluded by the epic itself).
- A real merchant-verification workflow change — the auto-verify behavior is a flag, off by default everywhere except the demo environment.
- Structured per-dish granular permissions/roles (that was the now-cancelled Employee Management Portal work).
- Push notifications, analytics, promotion-rule engines — excluded by the epic itself.
