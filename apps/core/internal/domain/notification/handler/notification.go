package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/notification/service"
	"github.com/gin-gonic/gin"
)

type NotificationHandler struct {
	svc *service.NotificationService
}

func NewNotificationHandler(svc *service.NotificationService) *NotificationHandler {
	if svc == nil {
		svc = service.NewNotificationService(nil)
	}
	return &NotificationHandler{svc: svc}
}

// ListNotifications godoc
// @Summary List notifications
// @Description Returns notifications for the authenticated user
// @Tags notification
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /notifications [get]
func (h *NotificationHandler) List(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	notifications, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list notifications"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": notifications})
}

// MarkNotificationRead godoc
// @Summary Mark notification as read
// @Description Marks a single notification as read
// @Tags notification
// @Produce json
// @Param id path int true "Notification ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /notifications/{id}/read [patch]
func (h *NotificationHandler) MarkRead(c *gin.Context) {
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

	notification, err := h.svc.MarkRead(c.Request.Context(), userID, id)
	if err != nil {
		if errors.Is(err, service.ErrNotificationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notification"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": notification})
}

// ReadAllNotifications godoc
// @Summary Mark all notifications as read
// @Description Marks all notifications as read for the authenticated user
// @Tags notification
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /notifications/read-all [post]
func (h *NotificationHandler) ReadAll(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.svc.ReadAll(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notifications"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
