package router

import (
	"net/http"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/admin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/ai"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/auth"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/category"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/conversation"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/coupon"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/dish"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/feed"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/media"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/merchant"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/merchants"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/notification"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/order"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/payment"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/review"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/stores"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/user"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/users"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/verification"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/voucher"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Setup registers all routes under the API base path. The base path is applied
// exactly once here, so it is the single source of truth for API versioning:
// domains declare paths relative to it and operational endpoints share it.
func Setup(router *gin.Engine, cfg *config.Config) {
	api := router.Group(cfg.Server.APIBasePath)

	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	api.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	auth.RegisterRoutes(api, cfg)
	ai.RegisterRoutes(api, cfg)
	user.RegisterRoutes(api, cfg)
	users.RegisterRoutes(api, cfg)
	coupon.RegisterRoutes(api, cfg)
	dish.RegisterRoutes(api, cfg)
	feed.RegisterRoutes(api, cfg)
	merchant.RegisterRoutes(api, cfg)
	merchants.RegisterRoutes(api, cfg)
	media.RegisterRoutes(api, cfg)
	payment.RegisterRoutes(api, cfg)
	review.RegisterRoutes(api, cfg)
	voucher.RegisterRoutes(api, cfg)
	stores.RegisterRoutes(api, cfg)
	category.RegisterRoutes(api, cfg)
	conversation.RegisterRoutes(api, cfg)
	notification.RegisterRoutes(api, cfg)
	verification.RegisterRoutes(api, cfg)
	admin.RegisterRoutes(api, cfg)
	order.RegisterRoutes(api, cfg)
}
