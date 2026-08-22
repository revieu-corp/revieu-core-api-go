package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/admin/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/admin/service"
)

// RegisterRoutes registers admin routes.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewAdminService(nil)
	h := handler.NewAdminHandler(svc)

	adminGroup := r.Group("/admin", authorization.JWTAuth(cfg.JWT), authorization.RequireRole("admin"))
	{
		adminGroup.GET("/reports", h.ListReports)
		adminGroup.PATCH("/reports/:id", h.UpdateReport)
		adminGroup.GET("/merchants", h.ListMerchants)
		adminGroup.PATCH("/merchants/:id", h.UpdateMerchant)
	}
}
