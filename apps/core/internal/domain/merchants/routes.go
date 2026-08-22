package merchants

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	followHandler "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/follow/handler"
	followService "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/follow/service"
	merchantHandler "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/merchant/handler"
	merchantService "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/merchant/service"
)

// RegisterRoutes consolidates all /merchants routes from merchant and follow
// domains. Single owner for the merchants resource prefix.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	merchantSvc := merchantService.NewMerchantService(nil)
	merchantH := merchantHandler.NewMerchantHandler(merchantSvc)

	followSvc := followService.NewFollowService(nil)
	followMerchantH := followHandler.NewMerchantHandler(followSvc)

	// Public: merchant list, detail, reviews
	merchantsPublic := r.Group("/merchants", authorization.OptionalJWTAuth(cfg.JWT))
	{
		merchantsPublic.GET("", merchantH.List)
		merchantsPublic.GET("/:id", merchantH.Detail)
		merchantsPublic.GET("/:id/reviews", merchantH.Reviews)
	}

	// Authenticated: follow/unfollow merchants
	merchantsAuth := r.Group("/merchants", authorization.JWTAuth(cfg.JWT))
	{
		merchantsAuth.POST("/:id/follow", followMerchantH.FollowMerchant)
		merchantsAuth.DELETE("/:id/follow", followMerchantH.UnfollowMerchant)
	}
}
