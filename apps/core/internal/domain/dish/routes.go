package dish

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish/service"
)

// RegisterRoutes registers the authenticated merchant menu API.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewDishService(nil)
	h := handler.NewDishHandler(svc)

	dishes := r.Group("/merchant/dishes", authorization.JWTAuth(cfg.JWT))
	{
		dishes.GET("", h.List)
		dishes.POST("", h.Create)
		dishes.PATCH("/:id", h.Update)
		dishes.DELETE("/:id", h.Delete)
		dishes.POST("/:id/enable", h.Enable)
		dishes.POST("/:id/disable", h.Disable)
	}
}
