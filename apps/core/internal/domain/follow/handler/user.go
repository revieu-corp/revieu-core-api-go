package handler

import (
	"net/http"
	"strconv"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/follow/service"
	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc *service.FollowService
}

func NewUserHandler(svc *service.FollowService) *UserHandler {
	if svc == nil {
		svc = service.NewFollowService(nil)
	}
	return &UserHandler{svc: svc}
}

// FollowUser godoc
// @Summary Follow user
// @Description Follow a user
// @Tags follow
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /users/{id}/follow [post]
func (h *UserHandler) FollowUser(c *gin.Context) {
	userID := c.GetInt64("user_id")
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := h.svc.FollowUser(c.Request.Context(), userID, targetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// UnfollowUser godoc
// @Summary Unfollow user
// @Description Unfollow a user
// @Tags follow
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /users/{id}/follow [delete]
func (h *UserHandler) UnfollowUser(c *gin.Context) {
	userID := c.GetInt64("user_id")
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := h.svc.UnfollowUser(c.Request.Context(), userID, targetID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
