package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/service"
)

type LikeHandler struct {
	svc *service.ContentService
}

func NewLikeHandler(svc *service.ContentService) *LikeHandler {
	if svc == nil {
		svc = service.NewContentService(nil)
	}
	return &LikeHandler{svc: svc}
}

// ListMyLikes godoc
// @Summary List my likes
// @Description Returns likes for the authenticated user
// @Tags content
// @Produce json
// @Param cursor query int false "Cursor"
// @Param limit query int false "Limit"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /user/likes [get]
func (h *LikeHandler) ListMyLikes(c *gin.Context) {
	userID := c.GetInt64("user_id")
	cursor, limit := parseCursorLimit(c)
	items, total, next, err := h.svc.ListLikes(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	respItems := make([]gin.H, 0, len(items))
	for _, item := range items {
		respItems = append(respItems, gin.H{
			"id":          item.ID,
			"target_type": item.TargetType,
			"target_id":   item.TargetID,
			"created_at":  item.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  respItems,
		"total":  total,
		"cursor": next,
	})
}
