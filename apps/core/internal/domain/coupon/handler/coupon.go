package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/coupon/service"
)

type CouponHandler struct {
	svc *service.CouponService
}

// InitiatePaymentRequest is the request body for initiating a coupon payment.
type InitiatePaymentRequest struct {
	UserID string `json:"userId" binding:"required"`
}

type CreateStoreCouponRequest struct {
	Title              string     `json:"title" binding:"required"`
	Description        string     `json:"description"`
	Type               string     `json:"type" binding:"required"`
	CouponType         string     `json:"coupon_type"`
	Price              float64    `json:"price"`
	OriginalPrice      *float64   `json:"original_price"`
	SalePrice          *float64   `json:"sale_price"`
	DiscountPercentage *float64   `json:"discount_percentage"`
	ImageURL           string     `json:"image_url"`
	DishIDs            []int64    `json:"dish_ids"`
	TotalQuantity      int        `json:"total_quantity" binding:"required"`
	MaxPerUser         int        `json:"max_per_user" binding:"required"`
	ValidFrom          *time.Time `json:"valid_from"`
	ValidUntil         *time.Time `json:"valid_until"`
	Terms              string     `json:"terms"`
	Status             string     `json:"status"`
}

type UpdateStoreCouponRequest struct {
	Title              *string    `json:"title"`
	Description        *string    `json:"description"`
	Type               *string    `json:"type"`
	CouponType         *string    `json:"coupon_type"`
	Price              *float64   `json:"price"`
	OriginalPrice      *float64   `json:"original_price"`
	SalePrice          *float64   `json:"sale_price"`
	DiscountPercentage *float64   `json:"discount_percentage"`
	ImageURL           *string    `json:"image_url"`
	DishIDs            *[]int64   `json:"dish_ids"`
	TotalQuantity      *int       `json:"total_quantity"`
	MaxPerUser         *int       `json:"max_per_user"`
	ValidFrom          *time.Time `json:"valid_from"`
	ValidUntil         *time.Time `json:"valid_until"`
	Terms              *string    `json:"terms"`
	Status             *string    `json:"status"`
}

type ValidateCouponRequest struct {
	Quantity int `json:"quantity"`
}

func NewCouponHandler(svc *service.CouponService) *CouponHandler {
	if svc == nil {
		svc = service.NewCouponService(nil)
	}
	return &CouponHandler{svc: svc}
}

// ListMerchantCoupons godoc
// @Summary List merchant coupons
// @Description Lists coupons owned by the authenticated active merchant
// @Tags coupon
// @Produce json
// @Param status query string false "Coupon status: draft, active, or disabled"
// @Param store_id query int false "Filter by owned store ID"
// @Param valid_from_before query string false "RFC3339 validity start upper bound"
// @Param valid_until_after query string false "RFC3339 validity end lower bound"
// @Param limit query int false "Page size (1-100)" default(20)
// @Param cursor query int false "ID cursor for the next page"
// @Success 200 {object} service.CouponPage
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/coupons [get]
func (h *CouponHandler) ListMerchantCoupons(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	query, err := parseMerchantCouponQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid coupon query"})
		return
	}
	page, err := h.svc.ListForMerchant(c.Request.Context(), userID, query)
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, page)
}

// UpdateMerchantCoupon godoc
// @Summary Update merchant coupon
// @Description Updates editable fields for an owned coupon; lifecycle changes use activate/deactivate
// @Tags coupon
// @Accept json
// @Produce json
// @Param id path int true "Coupon ID"
// @Param request body UpdateStoreCouponRequest true "Update coupon request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/coupons/{id} [patch]
func (h *CouponHandler) UpdateMerchantCoupon(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	couponID, err := parsePositiveID(c.Param("id"), "coupon")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req UpdateStoreCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid coupon payload"})
		return
	}
	coupon, err := h.svc.UpdateForMerchant(c.Request.Context(), userID, couponID, toUpdateStoreCouponInput(req))
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coupon})
}

// ActivateMerchantCoupon godoc
// @Summary Activate merchant coupon
// @Description Publishes an owned coupon for customer purchase
// @Tags coupon
// @Produce json
// @Param id path int true "Coupon ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/coupons/{id}/activate [post]
func (h *CouponHandler) ActivateMerchantCoupon(c *gin.Context) {
	h.setMerchantCouponStatus(c, "active")
}

// DeactivateMerchantCoupon godoc
// @Summary Deactivate merchant coupon
// @Description Hides an owned coupon from customer purchase
// @Tags coupon
// @Produce json
// @Param id path int true "Coupon ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/coupons/{id}/deactivate [post]
func (h *CouponHandler) DeactivateMerchantCoupon(c *gin.Context) {
	h.setMerchantCouponStatus(c, "disabled")
}

func (h *CouponHandler) setMerchantCouponStatus(c *gin.Context, status string) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	couponID, err := parsePositiveID(c.Param("id"), "coupon")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	coupon, err := h.svc.SetStatusForMerchant(c.Request.Context(), userID, couponID, status)
	if err != nil {
		statusCode, msg := couponErrorStatus(err)
		c.JSON(statusCode, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coupon})
}

func parseMerchantCouponQuery(c *gin.Context) (service.ListMerchantCouponsQuery, error) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return service.ListMerchantCouponsQuery{}, err
		}
		limit = parsed
	}
	cursor := int64(0)
	if raw := c.Query("cursor"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return service.ListMerchantCouponsQuery{}, err
		}
		cursor = parsed
	}
	query := service.ListMerchantCouponsQuery{Status: c.Query("status"), Limit: limit, Cursor: cursor}
	if raw := c.Query("store_id"); raw != "" {
		storeID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || storeID <= 0 {
			return service.ListMerchantCouponsQuery{}, errors.New("invalid store id")
		}
		query.StoreID = &storeID
	}
	if raw := c.Query("valid_from_before"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return service.ListMerchantCouponsQuery{}, err
		}
		query.ValidFromBefore = &value
	}
	if raw := c.Query("valid_until_after"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return service.ListMerchantCouponsQuery{}, err
		}
		query.ValidUntilAfter = &value
	}
	return query, nil
}

func parsePositiveID(raw, resource string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid " + resource + " id")
	}
	return id, nil
}

func toUpdateStoreCouponInput(req UpdateStoreCouponRequest) service.UpdateStoreCouponInput {
	return service.UpdateStoreCouponInput{
		Title: req.Title, Description: req.Description, Type: req.Type, CouponType: req.CouponType,
		Price: req.Price, OriginalPrice: req.OriginalPrice, SalePrice: req.SalePrice,
		DiscountPercentage: req.DiscountPercentage, ImageURL: req.ImageURL, DishIDs: req.DishIDs,
		TotalQuantity: req.TotalQuantity, MaxPerUser: req.MaxPerUser, ValidFrom: req.ValidFrom,
		ValidUntil: req.ValidUntil, Terms: req.Terms, Status: req.Status,
	}
}

// CreateStoreCoupon godoc
// @Summary Create store coupon
// @Description Creates a store-scoped coupon for an owned published store
// @Tags coupon
// @Accept json
// @Produce json
// @Param id path int true "Store ID"
// @Param request body CreateStoreCouponRequest true "Create store coupon request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id}/coupons [post]
func (h *CouponHandler) CreateStoreCoupon(c *gin.Context) {
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

	var req CreateStoreCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	coupon, err := h.svc.CreateForStore(c.Request.Context(), userID, storeID, service.CreateStoreCouponInput{
		Title: req.Title, Description: req.Description, Type: req.Type,
		CouponType: req.CouponType, Price: req.Price, OriginalPrice: req.OriginalPrice,
		SalePrice: req.SalePrice, DiscountPercentage: req.DiscountPercentage,
		ImageURL: req.ImageURL, DishIDs: req.DishIDs, TotalQuantity: req.TotalQuantity,
		MaxPerUser: req.MaxPerUser, ValidFrom: req.ValidFrom, ValidUntil: req.ValidUntil,
		Terms: req.Terms, Status: req.Status,
	})
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": coupon})
}

// ListMerchantStoreCoupons godoc
// @Summary List merchant store coupons
// @Description Lists all non-deleted coupons owned by the authenticated merchant for a store
// @Tags coupon
// @Produce json
// @Param id path int true "Store ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id}/coupons [get]
func (h *CouponHandler) ListMerchantStoreCoupons(c *gin.Context) {
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
	coupons, err := h.svc.ListForStore(c.Request.Context(), userID, storeID)
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coupons})
}

// UpdateStoreCoupon godoc
// @Summary Update merchant store coupon
// @Description Updates an owned store coupon
// @Tags coupon
// @Accept json
// @Produce json
// @Param id path int true "Store ID"
// @Param couponId path int true "Coupon ID"
// @Param request body UpdateStoreCouponRequest true "Update store coupon request"
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
	storeID, couponID, err := parseStoreCouponIDs(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req UpdateStoreCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	coupon, err := h.svc.UpdateForStore(c.Request.Context(), userID, storeID, couponID, service.UpdateStoreCouponInput{
		Title: req.Title, Description: req.Description, Type: req.Type, CouponType: req.CouponType,
		Price: req.Price, OriginalPrice: req.OriginalPrice, SalePrice: req.SalePrice,
		DiscountPercentage: req.DiscountPercentage, ImageURL: req.ImageURL, DishIDs: req.DishIDs,
		TotalQuantity: req.TotalQuantity, MaxPerUser: req.MaxPerUser, ValidFrom: req.ValidFrom,
		ValidUntil: req.ValidUntil, Terms: req.Terms, Status: req.Status,
	})
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coupon})
}

func (h *CouponHandler) EnableStoreCoupon(c *gin.Context) {
	h.setStoreCouponStatus(c, "active")
}

func (h *CouponHandler) DisableStoreCoupon(c *gin.Context) {
	h.setStoreCouponStatus(c, "disabled")
}

func (h *CouponHandler) setStoreCouponStatus(c *gin.Context, status string) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	storeID, couponID, err := parseStoreCouponIDs(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	coupon, err := h.svc.SetStatusForStore(c.Request.Context(), userID, storeID, couponID, status)
	if err != nil {
		statusCode, msg := couponErrorStatus(err)
		c.JSON(statusCode, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coupon})
}

func parseStoreCouponIDs(c *gin.Context) (int64, int64, error) {
	storeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid store id")
	}
	couponID, err := strconv.ParseInt(c.Param("couponId"), 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid coupon id")
	}
	return storeID, couponID, nil
}

// DeleteStoreCoupon godoc
// @Summary Delete store coupon
// @Description Soft-deletes a store-scoped coupon under an owned store
// @Tags coupon
// @Produce json
// @Param id path int true "Store ID"
// @Param couponId path int true "Coupon ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/stores/{id}/coupons/{couponId} [delete]
func (h *CouponHandler) DeleteStoreCoupon(c *gin.Context) {
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
	couponID, err := strconv.ParseInt(c.Param("couponId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid coupon id"})
		return
	}

	if err := h.svc.DeleteForStore(c.Request.Context(), userID, storeID, couponID); err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ListStoreCoupons godoc
// @Summary List store coupons
// @Description Lists published active coupons under a store
// @Tags coupon
// @Produce json
// @Param id path int true "Store ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Router /stores/{id}/coupons [get]
func (h *CouponHandler) ListStoreCoupons(c *gin.Context) {
	storeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid store id"})
		return
	}

	coupons, err := h.svc.ListPublishedByStore(c.Request.Context(), storeID)
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coupons})
}

// ValidateCoupon godoc
// @Summary Validate coupon
// @Description Validates a coupon by ID
// @Tags coupon
// @Accept json
// @Produce json
// @Param id path int true "Coupon ID"
// @Param request body ValidateCouponRequest false "Validate coupon request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /coupons/{id}/validate [post]
func (h *CouponHandler) Validate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req ValidateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.svc.Validate(c.Request.Context(), id, service.ValidateInput{
		Quantity: req.Quantity,
	})
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// InitiateCouponPayment godoc
// @Summary Initiate coupon payment
// @Description Initiates payment flow for a coupon
// @Tags coupon
// @Accept json
// @Produce json
// @Param id path int true "Coupon ID"
// @Param request body InitiatePaymentRequest true "Initiate payment request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /coupons/{id}/payment/initiate [post]
func (h *CouponHandler) InitiatePayment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req InitiatePaymentRequest
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.InitiatePayment(c.Request.Context(), id, req.UserID); err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// RedeemCoupon godoc
// @Summary Redeem coupon
// @Description Redeems a coupon for the authenticated user
// @Tags coupon
// @Produce json
// @Param id path int true "Coupon ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /coupons/{id}/redeem [post]
func (h *CouponHandler) Redeem(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID := c.GetInt64("user_id")
	if err := h.svc.Redeem(c.Request.Context(), id, userID); err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// ListPackages godoc
// @Summary List packages
// @Description Returns a list of available packages
// @Tags package
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /packages [get]
func (h *CouponHandler) ListPackages(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "not implemented"})
}

// PackageDetail godoc
// @Summary Get package detail
// @Description Returns a package by ID
// @Tags package
// @Produce json
// @Param id path int true "Package ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Router /packages/{id} [get]
func (h *CouponHandler) PackageDetail(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "not implemented"})
}

func couponErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrCouponNotFound), errors.Is(err, service.ErrStoreNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrStoreForbidden), errors.Is(err, service.ErrMerchantForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrCouponStoreMismatch):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrStoreNotPublished):
		return http.StatusBadRequest, "store not published"
	case errors.Is(err, service.ErrInvalidCouponInput):
		return http.StatusBadRequest, "invalid coupon input"
	case errors.Is(err, service.ErrCouponInactive):
		return http.StatusBadRequest, "coupon inactive"
	case errors.Is(err, service.ErrCouponExpired):
		return http.StatusBadRequest, "coupon expired"
	case errors.Is(err, service.ErrCouponNotStarted):
		return http.StatusBadRequest, "coupon not started"
	case errors.Is(err, service.ErrCouponSoldOut):
		return http.StatusBadRequest, "coupon sold out"
	case errors.Is(err, service.ErrCouponNotStoreScoped):
		return http.StatusBadRequest, "coupon must be store scoped"
	case errors.Is(err, service.ErrCouponStoreMismatch):
		return http.StatusBadRequest, "coupon store mismatch"
	case errors.Is(err, service.ErrCouponPerUserLimit):
		return http.StatusBadRequest, "coupon per-user limit exceeded"
	case errors.Is(err, service.ErrDeprecatedCouponRedeem), errors.Is(err, service.ErrDeprecatedCouponPayment):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
