package handler

import (
	"net/http"
	"strconv"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/follow/service"
	"github.com/gin-gonic/gin"
)

type MerchantHandler struct {
	svc *service.FollowService
}

func NewMerchantHandler(svc *service.FollowService) *MerchantHandler {
	if svc == nil {
		svc = service.NewFollowService(nil)
	}
	return &MerchantHandler{svc: svc}
}

// FollowMerchant godoc
// @Summary Follow merchant
// @Description Follow a merchant
// @Tags follow
// @Produce json
// @Param id path int true "Merchant ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /merchants/{id}/follow [post]
func (h *MerchantHandler) FollowMerchant(c *gin.Context) {
	userID := c.GetInt64("user_id")
	merchantID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant id"})
		return
	}
	if err := h.svc.FollowMerchant(c.Request.Context(), userID, merchantID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// UnfollowMerchant godoc
// @Summary Unfollow merchant
// @Description Unfollow a merchant
// @Tags follow
// @Produce json
// @Param id path int true "Merchant ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /merchants/{id}/follow [delete]
func (h *MerchantHandler) UnfollowMerchant(c *gin.Context) {
	userID := c.GetInt64("user_id")
	merchantID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant id"})
		return
	}
	if err := h.svc.UnfollowMerchant(c.Request.Context(), userID, merchantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
