package merchant

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/merchant/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/merchant/service"
)

// RegisterRoutes registers the current merchant's own account routes. Public
// /merchants routes live in the merchants domain, which owns that prefix.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewMerchantService(nil)
	h := handler.NewMerchantHandler(svc)

	// Authenticated: current merchant's account
	merchantPrivate := r.Group("/merchant", authorization.JWTAuth(cfg.JWT))
	{
		merchantPrivate.DELETE("/me", authorization.MerchantAccount(), h.DeleteMe)
	}
}
