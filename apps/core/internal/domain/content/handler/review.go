package handler

import (
	"net/http"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/service"
	"github.com/gin-gonic/gin"
)

type ReviewHandler struct {
	svc    *service.ContentService
	helper helper
}

func NewReviewHandler(svc *service.ContentService) *ReviewHandler {
	if svc == nil {
		svc = service.NewContentService(nil)
	}
	return &ReviewHandler{svc: svc, helper: newHelper()}
}

// ListUserReviews godoc
// @Summary List user's reviews
// @Description Returns a user's reviews
// @Tags content
// @Produce json
// @Param id path int true "User ID"
// @Param cursor query int false "Cursor"
// @Param limit query int false "Limit"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /users/{id}/reviews [get]
func (h *ReviewHandler) ListUserReviews(c *gin.Context) {
	targetID, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	cursor, limit := parseCursorLimit(c)
	reviews, total, nextCursor, err := h.svc.ListUserReviews(c.Request.Context(), targetID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	liked := h.helper.getLikedIDs(c, "review")
	items := make([]dto.ReviewItem, 0, len(reviews))
	for _, review := range reviews {
		merchant := h.helper.loadMerchantBrief(review.MerchantID)
		merchantValue := dto.MerchantBrief{}
		if merchant != nil {
			merchantValue = *merchant
		}
		items = append(items, dto.ReviewItem{
			ID:            review.ID,
			Rating:        review.Rating,
			RatingEnv:     review.RatingEnv,
			RatingService: review.RatingService,
			RatingValue:   review.RatingValue,
			Content:       review.Content,
			Images:        parseJSONStrings(review.Images),
			AvgCost:       review.AvgCost,
			LikeCount:     review.LikeCount,
			CommentCount:  review.CommentCount,
			IsLiked:       liked[review.ID],
			Merchant:      merchantValue,
			Tags:          []string{},
			CreatedAt:     review.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, dto.ReviewListResponse{Reviews: items, Total: int(total), Cursor: nextCursor})
}

// ListMyReviews godoc
// @Summary List my reviews
// @Description Returns reviews created by the authenticated user
// @Tags content
// @Produce json
// @Param cursor query int false "Cursor"
// @Param limit query int false "Limit"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /user/reviews [get]
func (h *ReviewHandler) ListMyReviews(c *gin.Context) {
	userID := c.GetInt64("user_id")
	cursor, limit := parseCursorLimit(c)
	reviews, total, nextCursor, err := h.svc.ListUserReviews(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	liked := h.helper.getLikedIDs(c, "review")
	items := make([]dto.ReviewItem, 0, len(reviews))
	for _, review := range reviews {
		merchant := h.helper.loadMerchantBrief(review.MerchantID)
		merchantValue := dto.MerchantBrief{}
		if merchant != nil {
			merchantValue = *merchant
		}
		items = append(items, dto.ReviewItem{
			ID:            review.ID,
			Rating:        review.Rating,
			RatingEnv:     review.RatingEnv,
			RatingService: review.RatingService,
			RatingValue:   review.RatingValue,
			Content:       review.Content,
			Images:        parseJSONStrings(review.Images),
			AvgCost:       review.AvgCost,
			LikeCount:     review.LikeCount,
			CommentCount:  review.CommentCount,
			IsLiked:       liked[review.ID],
			Merchant:      merchantValue,
			Tags:          []string{},
			CreatedAt:     review.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, dto.ReviewListResponse{Reviews: items, Total: int(total), Cursor: nextCursor})
}
