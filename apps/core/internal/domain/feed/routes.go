package feed

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/feed/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/feed/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/token"
)

// RegisterRoutes registers feed routes.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewFeedService(nil)
	h := handler.NewFeedHandler(svc)

	feed := r.Group("/feed", optionalViewerAuth(cfg))
	{
		feed.GET("/home", h.Home)
	}
}

// optionalViewerAuth keeps the documented public feed available to anonymous
// users while letting authenticated callers receive follow-scoped content.
func optionalViewerAuth(cfg *config.Config) gin.HandlerFunc {
	tokenService := token.New(cfg.JWT)
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.Next()
			return
		}
		claims, err := tokenService.ValidateToken(strings.TrimPrefix(header, "Bearer "))
		if err == nil {
			if sub, ok := claims["sub"].(float64); ok {
				c.Set("user_id", int64(sub))
			}
		}
		c.Next()
	}
}
