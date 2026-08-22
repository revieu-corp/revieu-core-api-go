package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/voucher/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/voucher/service"
	"github.com/gin-gonic/gin"
)

type VoucherHandler struct {
	svc         *service.VoucherService
	frontendURL string
}

type RedeemByTokenRequest struct {
	ScanToken string `json:"scan_token"`
}

func NewVoucherHandler(svc *service.VoucherService, frontendURL string) *VoucherHandler {
	if svc == nil {
		svc = service.NewVoucherService(nil)
	}
	return &VoucherHandler{
		svc:         svc,
		frontendURL: frontendURL,
	}
}

// CreateVoucher godoc
// @Summary Create voucher
// @Description Creates a voucher for the authenticated user
// @Tags voucher
// @Accept json
// @Produce json
// @Param request body service.CreateVoucherRequest true "Create voucher request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /vouchers [post]
func (h *VoucherHandler) Create(c *gin.Context) {
	var req service.CreateVoucherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}

// ListVouchers godoc
// @Summary List vouchers
// @Description Returns vouchers for the authenticated user
// @Tags voucher
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /vouchers [get]
func (h *VoucherHandler) List(c *gin.Context) {
	userID := c.GetInt64("user_id")
	list, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// VoucherDetail godoc
// @Summary Get voucher detail
// @Description Returns a voucher by ID
// @Tags voucher
// @Produce json
// @Param id path int true "Voucher ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /vouchers/{id} [get]
func (h *VoucherHandler) Detail(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	v, err := h.svc.DetailForUser(c.Request.Context(), userID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	resp, err := dto.FromModel(*v, h.frontendURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build scan url"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// VoucherByCode godoc
// @Summary Get voucher by code
// @Description Returns a voucher by code
// @Tags voucher
// @Produce json
// @Param code path string true "Voucher code"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /vouchers/code/{code} [get]
func (h *VoucherHandler) ByCode(c *gin.Context) {
	userID := c.GetInt64("user_id")
	v, err := h.svc.ByCodeForUser(c.Request.Context(), userID, c.Param("code"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	resp, err := dto.FromModel(*v, h.frontendURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build scan url"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// UseVoucher godoc
// @Summary Use voucher
// @Description Marks a voucher as used
// @Tags voucher
// @Produce json
// @Param id path int true "Voucher ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /vouchers/{id}/use [patch]
func (h *VoucherHandler) Use(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.Use(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// UpdateVoucherStatus godoc
// @Summary Update voucher status
// @Description Updates voucher status to used
// @Tags voucher
// @Produce json
// @Param id path int true "Voucher ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /vouchers/{id}/status [patch]
func (h *VoucherHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.UpdateStatus(c.Request.Context(), id, "used"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// ShareVoucherEmail godoc
// @Summary Share voucher via email
// @Description Sends voucher share email
// @Tags voucher
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /vouchers/share/email [post]
func (h *VoucherHandler) ShareEmail(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) }

// ShareVoucherSMS godoc
// @Summary Share voucher via SMS
// @Description Sends voucher share SMS
// @Tags voucher
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /vouchers/share/sms [post]
func (h *VoucherHandler) ShareSMS(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) }

// MerchantVoucherScanPreview godoc
// @Summary Preview voucher redemption by scan token
// @Description Validates a scanned voucher for the authenticated merchant without mutating state
// @Tags voucher
// @Produce json
// @Param t query string true "Voucher scan token"
// @Success 200 {object} service.RedeemPreview
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/vouchers/scan [get]
func (h *VoucherHandler) ScanPreview(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	scanToken := strings.TrimSpace(c.Query("t"))
	if scanToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing scan token"})
		return
	}

	preview, err := h.svc.PreviewRedeemByToken(c.Request.Context(), userID, scanToken)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrVoucherForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, service.ErrVoucherNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to preview voucher"})
		}
		return
	}

	c.JSON(http.StatusOK, preview)
}

// RedeemVoucherByToken godoc
// @Summary Redeem voucher by scan token
// @Description Redeems a scanned voucher for the authenticated merchant
// @Tags voucher
// @Accept json
// @Produce json
// @Param request body handler.RedeemByTokenRequest true "Redeem by token request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/vouchers/redeem-by-token [post]
func (h *VoucherHandler) RedeemByToken(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req RedeemByTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ScanToken = strings.TrimSpace(req.ScanToken)
	if req.ScanToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing scan token"})
		return
	}

	if err := h.svc.RedeemByMerchantToken(c.Request.Context(), userID, req.ScanToken); err != nil {
		switch {
		case errors.Is(err, service.ErrVoucherNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, service.ErrVoucherForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, service.ErrVoucherNotRedeemable):
			c.JSON(http.StatusBadRequest, gin.H{"error": "voucher not redeemable"})
		case errors.Is(err, service.ErrVoucherExpired):
			c.JSON(http.StatusBadRequest, gin.H{"error": "voucher expired"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to redeem voucher"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// RedeemVoucherByMerchant godoc
// @Summary Redeem voucher by merchant owner
// @Description Redeems a voucher for merchant-owned store operations
// @Tags voucher
// @Produce json
// @Param id path int true "Voucher ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/vouchers/{id}/redeem [post]
func (h *VoucherHandler) RedeemByMerchant(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.RedeemByMerchant(c.Request.Context(), userID, id); err != nil {
		switch {
		case errors.Is(err, service.ErrVoucherNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, service.ErrVoucherForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, service.ErrVoucherNotRedeemable):
			c.JSON(http.StatusBadRequest, gin.H{"error": "voucher not redeemable"})
		case errors.Is(err, service.ErrVoucherExpired):
			c.JSON(http.StatusBadRequest, gin.H{"error": "voucher expired"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to redeem voucher"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
