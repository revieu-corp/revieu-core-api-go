package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/service"
)

type MerchantReviewHandler struct {
	svc *service.ReviewService
}

func NewMerchantReviewHandler(svc *service.ReviewService) *MerchantReviewHandler {
	if svc == nil {
		svc = service.NewReviewService(nil)
	}
	return &MerchantReviewHandler{svc: svc}
}

// ListMerchantReviews godoc
// @Summary List current merchant reviews
// @Description Returns active reviews belonging to the authenticated merchant
// @Tags merchant-review
// @Produce json
// @Success 200 {object} dto.MerchantReviewListResponse
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/reviews [get]
func (h *MerchantReviewHandler) List(c *gin.Context) {
	items, err := h.svc.ListMerchantReviews(c.Request.Context(), c.GetInt64("user_id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.MerchantReviewListResponse{Data: items})
}

// Reply godoc
// @Summary Reply to a merchant review
// @Description Creates or updates the authenticated merchant's reply to a review
// @Tags merchant-review
// @Accept json
// @Produce json
// @Param id path int true "Review ID"
// @Param request body dto.MerchantReplyRequest true "Reply request"
// @Success 200 {object} dto.MerchantReview
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 422 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/reviews/{id}/reply [post]
func (h *MerchantReviewHandler) Reply(c *gin.Context) {
	reviewID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review id"})
		return
	}
	var req dto.MerchantReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "reply text must be between 1 and 500 characters"})
		return
	}
	item, err := h.svc.ReplyToMerchantReview(c.Request.Context(), c.GetInt64("user_id"), reviewID, req.Text)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// Delete godoc
// @Summary Archive a merchant review
// @Description Archives an active review owned by the authenticated merchant
// @Tags merchant-review
// @Produce json
// @Param id path int true "Review ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /merchant/reviews/{id} [delete]
func (h *MerchantReviewHandler) Delete(c *gin.Context) {
	reviewID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review id"})
		return
	}
	if err := h.svc.DeleteMerchantReview(c.Request.Context(), c.GetInt64("user_id"), reviewID); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *MerchantReviewHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidReply):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrReviewForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrReviewNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrMerchantNotFound):
		c.JSON(http.StatusForbidden, gin.H{"error": "authenticated user is not a merchant"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "merchant review operation failed"})
	}
}
