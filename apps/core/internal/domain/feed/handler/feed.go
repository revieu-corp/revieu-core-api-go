package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/feed/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/feed/service"
)

type FeedHandler struct {
	svc *service.FeedService
}

func NewFeedHandler(svc *service.FeedService) *FeedHandler {
	if svc == nil {
		svc = service.NewFeedService(nil)
	}
	return &FeedHandler{svc: svc}
}

// HomeFeed godoc
// @Summary Get home feed
// @Description Returns the home feed items
// @Tags feed
// @Produce json
// @Param cursor query string false "Opaque cursor for the next page"
// @Param limit query int false "Page size (1-100, default 20)"
// @Success 200 {object} dto.FeedResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /feed/home [get]
func (h *FeedHandler) Home(c *gin.Context) {
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
			return
		}
		limit = parsed
	}
	items, cursor, err := h.svc.Home(c.Request.Context(), c.GetInt64("user_id"), c.Query("cursor"), limit)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCursor) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.FeedResponse{Data: items, Cursor: cursor})
}
