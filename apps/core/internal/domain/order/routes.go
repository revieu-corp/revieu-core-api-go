package order

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/order/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/order/service"
)

// RegisterRoutes registers order routes.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewOrderServiceForMode(nil, cfg.Server.Mode)
	h := handler.NewOrderHandler(svc)

	orders := r.Group("/orders", authorization.JWTAuth(cfg.JWT))
	{
		orders.POST("", h.Create)
		orders.GET("", h.List)
		orders.GET("/:id", h.Detail)
		orders.POST("/:id/pay", h.Pay)
	}
}
