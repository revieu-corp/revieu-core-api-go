package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
)

// RegisterRoutes registers auth routes: public endpoints for login/register,
// authenticated endpoints for profile access.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	handler := NewHandler(cfg.JWT, cfg.OAuth, cfg.SMTP, cfg.FrontendURL, cfg.Server.APIBasePath)

	// Public: registration, login, password reset, OAuth callbacks, email verification
	authPublic := r.Group("/auth")
	{
		authPublic.POST("/register", handler.Register)
		authPublic.POST("/login", handler.Login)
		authPublic.POST("/refresh", handler.Refresh)
		authPublic.POST("/logout", handler.Logout)
		authPublic.POST("/forgot-password", handler.ForgotPassword)
		authPublic.POST("/reset-password", handler.ResetPassword)
		authPublic.GET("/login/google", handler.GoogleLogin)
		authPublic.GET("/callback/google", handler.GoogleCallback)
		authPublic.GET("/verify", handler.VerifyEmail)
	}

	// Authenticated: current user profile
	authPrivate := r.Group("/auth", authorization.JWTAuth(cfg.JWT))
	{
		authPrivate.GET("/me", handler.Me)
	}
}
