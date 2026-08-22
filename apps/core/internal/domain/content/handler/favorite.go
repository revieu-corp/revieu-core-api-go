package handler

import (
	"net/http"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/gin-gonic/gin"
)

type FavoriteHandler struct {
	svc    *service.ContentService
	helper helper
}

func NewFavoriteHandler(svc *service.ContentService) *FavoriteHandler {
	if svc == nil {
		svc = service.NewContentService(nil)
	}
	return &FavoriteHandler{svc: svc, helper: newHelper()}
}

// ListMyFavorites godoc
// @Summary List my favorites
// @Description Returns favorites for the authenticated user
// @Tags content
// @Produce json
// @Param type query string false "Target type (post|review|merchant)"
// @Param cursor query int false "Cursor"
// @Param limit query int false "Limit"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /user/favorites [get]
func (h *FavoriteHandler) ListMyFavorites(c *gin.Context) {
	userID := c.GetInt64("user_id")
	targetType := c.Query("type")
	cursor, limit := parseCursorLimit(c)

	items, total, err := h.svc.ListFavorites(c.Request.Context(), userID, targetType, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	respItems := make([]dto.FavoriteItem, 0, len(items))
	for _, item := range items {
		fav := dto.FavoriteItem{
			ID:         item.ID,
			TargetType: item.TargetType,
			TargetID:   item.TargetID,
			CreatedAt:  item.CreatedAt,
		}
		switch item.TargetType {
		case "post":
			var post model.Post
			if err := h.helper.db.WithContext(c.Request.Context()).First(&post, item.TargetID).Error; err == nil {
				fav.Post = &dto.PostItem{
					ID:        post.ID,
					Title:     post.Title,
					Content:   post.Content,
					Images:    parseJSONStrings(post.Images),
					LikeCount: post.LikeCount,
					ViewCount: post.ViewCount,
					IsLiked:   false,
					Merchant:  nil,
					Tags:      []string{},
					CreatedAt: post.CreatedAt,
				}
			}
		case "review":
			var review model.Review
			if err := h.helper.db.WithContext(c.Request.Context()).First(&review, item.TargetID).Error; err == nil {
				fav.Review = &dto.ReviewItem{
					ID:            review.ID,
					Rating:        review.Rating,
					RatingEnv:     review.RatingEnv,
					RatingService: review.RatingService,
					RatingValue:   review.RatingValue,
					Content:       review.Content,
					Images:        parseJSONStrings(review.Images),
					AvgCost:       review.AvgCost,
					LikeCount:     review.LikeCount,
					IsLiked:       false,
					Merchant:      dto.MerchantBrief{},
					Tags:          []string{},
					CreatedAt:     review.CreatedAt,
				}
			}
		case "merchant":
			if merchant := h.helper.loadMerchantBrief(item.TargetID); merchant != nil {
				fav.Merchant = merchant
			}
		}
		respItems = append(respItems, fav)
	}

	resp := dto.FavoriteListResponse{Items: respItems, Total: int(total), Cursor: nextCursor(items)}
	c.JSON(http.StatusOK, resp)
}
