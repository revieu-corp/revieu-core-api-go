package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/verification/service"
)

type VerificationHandler struct {
	svc *service.VerificationService
}

func NewVerificationHandler(svc *service.VerificationService) *VerificationHandler {
	if svc == nil {
		svc = service.NewVerificationService(nil)
	}
	return &VerificationHandler{svc: svc}
}

// SubmitVerification godoc
// @Summary Submit merchant verification
// @Description Submits verification documents for the authenticated merchant
// @Tags verification
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /merchant/verification [post]
func (h *VerificationHandler) Submit(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req service.SubmitVerificationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	verification, err := h.svc.Submit(c.Request.Context(), userID, req)
	if err != nil {
		if errors.Is(err, service.ErrVerificationForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "merchant access required"})
			return
		}
		if errors.Is(err, service.ErrVerificationInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid verification input"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit verification"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": verification})
}

// VerificationStatus godoc
// @Summary Get verification status
// @Description Returns the verification status for the authenticated merchant
// @Tags verification
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /merchant/verification [get]
func (h *VerificationHandler) Status(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	status, err := h.svc.Status(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrVerificationForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "merchant access required"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load verification status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": status})
}
