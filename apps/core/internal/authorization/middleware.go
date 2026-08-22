package authorization

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/token"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
)

const (
	AuthorizationHeader   = "Authorization"
	BearerPrefix          = "Bearer "
	AccessTokenCookieName = "revieu_access_token"
	UserIDKey             = "user_id"
	UserEmailKey          = "user_email"
	UserRoleKey           = "user_role"
)

// JWTAuth authenticates a bearer token and installs its principal in Gin context.
func JWTAuth(jwtCfg config.JWTConfig) gin.HandlerFunc {
	tokenService := token.New(jwtCfg)
	return func(c *gin.Context) {
		rawToken := ""
		if authHeader := c.GetHeader(AuthorizationHeader); authHeader != "" {
			if !strings.HasPrefix(authHeader, BearerPrefix) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
				return
			}
			rawToken = strings.TrimPrefix(authHeader, BearerPrefix)
		} else if cookieToken, err := c.Cookie(AccessTokenCookieName); err == nil {
			rawToken = cookieToken
		}
		if rawToken == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization credentials"})
			return
		}

		claims, err := tokenService.ValidateToken(rawToken)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		sub, ok := claims["sub"].(float64)
		if !ok || sub <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token subject"})
			return
		}
		userID := int64(sub)
		// Access tokens are short-lived, but a disabled user must stop being
		// accepted immediately rather than retaining access until expiry. The
		// nil check keeps router-only unit tests usable before a DB is connected.
		if database.DB != nil {
			var user model.User
			if err := database.DB.Select("id", "status").First(&user, userID).Error; err != nil || user.Status != 0 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user is inactive"})
				return
			}
		}
		c.Set(UserIDKey, userID)
		if email, ok := claims["email"].(string); ok {
			c.Set(UserEmailKey, email)
		}
		if role, ok := claims["role"].(string); ok {
			c.Set(UserRoleKey, role)
		}
		c.Next()
	}
}
