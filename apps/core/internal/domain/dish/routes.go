package dish

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish/service"
)

// RegisterRoutes registers merchant-private dish management routes.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewDishService(nil)
	h := handler.NewDishHandler(svc)

	dishes := r.Group("/merchant/dishes", authorization.JWTAuth(cfg.JWT))
	{
		dishes.POST("", authorization.VerifiedMerchant(), h.Create)
		dishes.GET("", authorization.MerchantAccount(), h.ListMine)
		dishes.PATCH("/:id", authorization.VerifiedMerchant(), h.Update)
		dishes.DELETE("/:id", authorization.VerifiedMerchant(), h.Delete)
		dishes.POST("/:id/enable", authorization.VerifiedMerchant(), h.Enable)
		dishes.POST("/:id/disable", authorization.VerifiedMerchant(), h.Disable)
	}
}
