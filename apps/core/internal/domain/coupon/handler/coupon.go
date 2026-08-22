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
	Title         string     `json:"title" binding:"required"`
	Description   string     `json:"description"`
	Type          string     `json:"type" binding:"required"`
	Price         float64    `json:"price"`
	TotalQuantity int        `json:"total_quantity" binding:"required"`
	MaxPerUser    int        `json:"max_per_user" binding:"required"`
	ValidFrom     *time.Time `json:"valid_from"`
	ValidUntil    *time.Time `json:"valid_until"`
	Terms         string     `json:"terms"`
	Status        string     `json:"status"`
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
		Title:         req.Title,
		Description:   req.Description,
		Type:          req.Type,
		Price:         req.Price,
		TotalQuantity: req.TotalQuantity,
		MaxPerUser:    req.MaxPerUser,
		ValidFrom:     req.ValidFrom,
		ValidUntil:    req.ValidUntil,
		Terms:         req.Terms,
		Status:        req.Status,
	})
	if err != nil {
		status, msg := couponErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": coupon})
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
// @Description Returns a paginated list of active packages with active coupons
// @Tags package
// @Produce json
// @Param limit query int false "Page size (1-100)" default(20)
// @Param cursor query int false "ID cursor for the next page"
// @Success 200 {object} service.PackagePage
// @Failure 400 {object} map[string]string
// @Router /packages [get]
func (h *CouponHandler) ListPackages(c *gin.Context) {
	limit, cursor, err := parsePackagePageQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid package query"})
		return
	}

	page, err := h.svc.ListPackages(c.Request.Context(), service.ListPackagesQuery{Limit: limit, Cursor: cursor})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCouponInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid package query"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list packages"})
		return
	}
	c.JSON(http.StatusOK, page)
}

// PackageDetail godoc
// @Summary Get package detail
// @Description Returns a package by ID
// @Tags package
// @Produce json
// @Param id path int true "Package ID"
// @Success 200 {object} service.PackageResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /packages/{id} [get]
func (h *CouponHandler) PackageDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid package id"})
		return
	}

	pkg, err := h.svc.PackageDetail(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrPackageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "package not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load package"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": pkg})
}

func parsePackagePageQuery(c *gin.Context) (int, int64, error) {
	limit := 0
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			return 0, 0, err
		}
		limit = parsed
	}

	cursor := int64(0)
	if rawCursor := c.Query("cursor"); rawCursor != "" {
		parsed, err := strconv.ParseInt(rawCursor, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		cursor = parsed
	}
	return limit, cursor, nil
}

func couponErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrCouponNotFound), errors.Is(err, service.ErrStoreNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrStoreForbidden):
		return http.StatusForbidden, "forbidden"
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
