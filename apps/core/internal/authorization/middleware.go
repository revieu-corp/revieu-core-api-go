package authorization

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/token"
)

const (
	AuthorizationHeader = "Authorization"
	BearerPrefix        = "Bearer "
	UserIDKey           = "user_id"
	UserEmailKey        = "user_email"
	UserRoleKey         = "user_role"
)

// JWTAuth authenticates a bearer token and installs its principal in Gin context.
func JWTAuth(jwtCfg config.JWTConfig) gin.HandlerFunc {
	tokenService := token.New(jwtCfg)
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthorizationHeader)
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}
		if !strings.HasPrefix(authHeader, BearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}
		claims, err := tokenService.ValidateToken(strings.TrimPrefix(authHeader, BearerPrefix))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		if sub, ok := claims["sub"].(float64); ok {
			c.Set(UserIDKey, int64(sub))
		}
		if email, ok := claims["email"].(string); ok {
			c.Set(UserEmailKey, email)
		}
		if role, ok := claims["role"].(string); ok {
			c.Set(UserRoleKey, role)
		}
		c.Next()
	}
}

// OptionalJWTAuth installs the authenticated principal when a valid bearer
// token is present, while keeping public routes available to anonymous users.
// Invalid or expired optional credentials are treated as anonymous so they do
// not bypass the public privacy policy or turn a public read into a 401.
func OptionalJWTAuth(jwtCfg config.JWTConfig) gin.HandlerFunc {
	tokenService := token.New(jwtCfg)
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthorizationHeader)
		if authHeader == "" || !strings.HasPrefix(authHeader, BearerPrefix) {
			c.Next()
			return
		}

		claims, err := tokenService.ValidateToken(strings.TrimPrefix(authHeader, BearerPrefix))
		if err == nil {
			if sub, ok := claims["sub"].(float64); ok {
				c.Set(UserIDKey, int64(sub))
			}
			if email, ok := claims["email"].(string); ok {
				c.Set(UserEmailKey, email)
			}
			if role, ok := claims["role"].(string); ok {
				c.Set(UserRoleKey, role)
			}
		}
		c.Next()
	}
}
