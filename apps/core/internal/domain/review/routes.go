package review

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review/service"
)

// RegisterRoutes registers review routes: public reads and authenticated writes.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewReviewService(nil)
	h := handler.NewReviewHandler(svc)
	merchantH := handler.NewMerchantReviewHandler(svc)

	// Public: anyone can read review details
	reviewsPublic := r.Group("/reviews")
	{
		reviewsPublic.GET("/:id", h.Detail)
	}

	// Authenticated: create reviews, like, and comment
	reviewsAuth := r.Group("/reviews", authorization.JWTAuth(cfg.JWT))
	{
		reviewsAuth.POST("", h.Create)
		reviewsAuth.POST("/:id/like", h.Like)
		reviewsAuth.POST("/:id/comments", h.Comment)
	}

	// Authenticated merchant dashboard review workflow. The merchant is
	// resolved from the JWT user id; clients cannot choose another merchant.
	merchantReviews := r.Group("/merchant", authorization.JWTAuth(cfg.JWT))
	{
		merchantReviews.GET("/reviews", merchantH.List)
		merchantReviews.POST("/reviews/:id/reply", merchantH.Reply)
		merchantReviews.DELETE("/reviews/:id", merchantH.Delete)
	}
}
