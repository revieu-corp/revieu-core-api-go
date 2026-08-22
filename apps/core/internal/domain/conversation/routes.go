package conversation

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/conversation/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/conversation/service"
)

// RegisterRoutes registers conversation routes.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewConversationService(nil)
	h := handler.NewConversationHandler(svc)

	convos := r.Group("/conversations", authorization.JWTAuth(cfg.JWT))
	{
		convos.GET("", h.List)
		convos.POST("", h.Create)
		convos.DELETE("/:id", h.Delete)
		convos.GET("/:id/messages", h.Messages)
		convos.POST("/:id/messages", h.SendMessage)
		convos.DELETE("/:id/messages", h.ClearMessages)
		convos.PATCH("/:id/settings", h.UpdateSettings)
	}
}
