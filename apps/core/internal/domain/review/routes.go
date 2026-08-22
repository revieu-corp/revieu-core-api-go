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

	// Public: anyone can read review details
	reviewsPublic := r.Group("/reviews", authorization.OptionalJWTAuth(cfg.JWT))
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
}
