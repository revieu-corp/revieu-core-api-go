package handler

import (
	"net/http"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/service"
	"github.com/gin-gonic/gin"
)

type PostHandler struct {
	svc    *service.ContentService
	helper helper
}

func NewPostHandler(svc *service.ContentService) *PostHandler {
	if svc == nil {
		svc = service.NewContentService(nil)
	}
	return &PostHandler{svc: svc, helper: newHelper()}
}

// ListUserPosts godoc
// @Summary List user's posts
// @Description Returns a user's posts
// @Tags content
// @Produce json
// @Param id path int true "User ID"
// @Param cursor query int false "Cursor"
// @Param limit query int false "Limit"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /users/{id}/posts [get]
func (h *PostHandler) ListUserPosts(c *gin.Context) {
	targetID, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	cursor, limit := parseCursorLimit(c)
	posts, total, err := h.svc.ListUserPosts(c.Request.Context(), targetID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	liked := h.helper.getLikedIDs(c, "post")
	items := make([]dto.PostItem, 0, len(posts))
	for _, post := range posts {
		var merchant *dto.MerchantBrief
		if post.MerchantID != nil {
			merchant = h.helper.loadMerchantBrief(*post.MerchantID)
		}
		items = append(items, dto.PostItem{
			ID:        post.ID,
			Title:     post.Title,
			Content:   post.Content,
			Images:    parseJSONStrings(post.Images),
			LikeCount: post.LikeCount,
			ViewCount: post.ViewCount,
			IsLiked:   liked[post.ID],
			Merchant:  merchant,
			Tags:      []string{},
			CreatedAt: post.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, dto.PostListResponse{Posts: items, Total: int(total), Cursor: nextCursor(posts)})
}

// ListMyPosts godoc
// @Summary List my posts
// @Description Returns posts created by the authenticated user
// @Tags content
// @Produce json
// @Param cursor query int false "Cursor"
// @Param limit query int false "Limit"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /user/posts [get]
func (h *PostHandler) ListMyPosts(c *gin.Context) {
	userID := c.GetInt64("user_id")
	cursor, limit := parseCursorLimit(c)
	posts, total, err := h.svc.ListUserPosts(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	liked := h.helper.getLikedIDs(c, "post")
	items := make([]dto.PostItem, 0, len(posts))
	for _, post := range posts {
		var merchant *dto.MerchantBrief
		if post.MerchantID != nil {
			merchant = h.helper.loadMerchantBrief(*post.MerchantID)
		}
		items = append(items, dto.PostItem{
			ID:        post.ID,
			Title:     post.Title,
			Content:   post.Content,
			Images:    parseJSONStrings(post.Images),
			LikeCount: post.LikeCount,
			ViewCount: post.ViewCount,
			IsLiked:   liked[post.ID],
			Merchant:  merchant,
			Tags:      []string{},
			CreatedAt: post.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, dto.PostListResponse{Posts: items, Total: int(total), Cursor: nextCursor(posts)})
}
