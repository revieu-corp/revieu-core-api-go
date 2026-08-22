package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/merchant/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/merchant/service"
	reviewdto "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/dto"
)

type MerchantHandler struct {
	svc *service.MerchantService
}

func NewMerchantHandler(svc *service.MerchantService) *MerchantHandler {
	if svc == nil {
		svc = service.NewMerchantService(nil)
	}
	return &MerchantHandler{svc: svc}
}

// ListMerchants godoc
// @Summary List merchants
// @Description Returns merchants, optionally filtered by category
// @Tags merchant
// @Produce json
// @Param category query string false "Category filter"
// @Param search query string false "Search by name"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /merchants [get]
func (h *MerchantHandler) List(c *gin.Context) {
	category := c.Query("category")
	search := c.Query("search")
	merchants, err := h.svc.List(c.Request.Context(), category, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]dto.Merchant, 0, len(merchants))
	for _, m := range merchants {
		items = append(items, dto.FromModel(m))
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// MerchantDetail godoc
// @Summary Get merchant detail
// @Description Returns a merchant by ID
// @Tags merchant
// @Produce json
// @Param id path int true "Merchant ID"
// @Success 200 {object} dto.Merchant
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /merchants/{id} [get]
func (h *MerchantHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	merchant, err := h.svc.Detail(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrMerchantNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, dto.FromModel(*merchant))
}

// MerchantReviews godoc
// @Summary List merchant reviews
// @Description Returns reviews for a merchant
// @Tags merchant
// @Produce json
// @Param id path int true "Merchant ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /merchants/{id}/reviews [get]
func (h *MerchantHandler) Reviews(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	reviews, err := h.svc.ReviewsForViewer(c.Request.Context(), id, c.GetInt64("user_id"))
	if err != nil {
		if errors.Is(err, service.ErrMerchantNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reviewdto.FromModels(reviews)})
}

// DeleteMerchantMe godoc
// @Summary Delete current merchant (placeholder)
// @Description Placeholder endpoint for future merchant deletion flow
// @Tags merchant
// @Produce json
// @Success 501 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/me [delete]
func (h *MerchantHandler) DeleteMe(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
