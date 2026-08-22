package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/visibility"
)

type ReviewHandler struct {
	svc *service.ReviewService
}

func NewReviewHandler(svc *service.ReviewService) *ReviewHandler {
	if svc == nil {
		svc = service.NewReviewService(nil)
	}
	return &ReviewHandler{svc: svc}
}

// CreateReview godoc
// @Summary Create a review
// @Description Creates a new review for the authenticated user
// @Tags review
// @Accept json
// @Produce json
// @Param request body dto.Review true "Create review request"
// @Success 201 {object} dto.Review
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 422 {object} map[string]string
// @Router /reviews [post]
func (h *ReviewHandler) Create(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.Review
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMerchantNotFound), errors.Is(err, service.ErrStoreNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrStoreMerchantMismatch):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, dto.FromModel(created))
}

// ReviewDetail godoc
// @Summary Get review detail
// @Description Returns a single review by ID
// @Tags review
// @Produce json
// @Param id path int true "Review ID"
// @Success 200 {object} dto.Review
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /reviews/{id} [get]
func (h *ReviewHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	review, err := h.svc.DetailForViewer(c.Request.Context(), id, c.GetInt64("user_id"))
	if err != nil {
		if errors.Is(err, visibility.ErrPrivateContent) {
			c.JSON(http.StatusForbidden, gin.H{"error": "content is private"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, dto.FromModel(*review))
}

// LikeReview godoc
// @Summary Like a review
// @Description Likes a review for the authenticated user
// @Tags review
// @Produce json
// @Param id path int true "Review ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /reviews/{id}/like [post]
func (h *ReviewHandler) Like(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID := c.GetInt64("user_id")
	if err := h.svc.Like(c.Request.Context(), userID, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// CommentReview godoc
// @Summary Add a review comment
// @Description Adds a comment to a review
// @Tags review
// @Accept json
// @Produce json
// @Param id path int true "Review ID"
// @Param request body dto.CommentRequest true "Comment request"
// @Success 201 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /reviews/{id}/comments [post]
func (h *ReviewHandler) Comment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID := c.GetInt64("user_id")
	var req dto.CommentRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid text"})
		return
	}
	if err := h.svc.Comment(c.Request.Context(), userID, id, req.Text); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{})
}
