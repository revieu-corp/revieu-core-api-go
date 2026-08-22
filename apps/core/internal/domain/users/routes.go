package users

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	contentHandler "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/handler"
	contentService "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/service"
	followHandler "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/follow/handler"
	followService "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/follow/service"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/profile"
	profileService "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/profile/service"
)

// RegisterRoutes consolidates all /users routes from profile, content, and follow
// domains. This domain does not implement handlers; it delegates to the domains
// that do, ensuring /users has a single owner for route registration.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	// Profile handlers
	profileSvc := profileService.NewService(nil)
	profileH := profile.NewHandler(profileSvc)

	// Content handlers
	contentSvc := contentService.NewContentService(nil)
	postH := contentHandler.NewPostHandler(contentSvc)
	reviewH := contentHandler.NewReviewHandler(contentSvc)

	// Follow handlers
	followSvc := followService.NewFollowService(nil)
	followUserH := followHandler.NewUserHandler(followSvc)

	// Public: user profiles and content
	usersPublic := r.Group("/users", authorization.OptionalJWTAuth(cfg.JWT))
	{
		usersPublic.GET("/:id", profileH.GetPublicProfile)
		usersPublic.GET("/:id/posts", postH.ListUserPosts)
		usersPublic.GET("/:id/reviews", reviewH.ListUserReviews)
	}

	// Authenticated: follow/unfollow users
	usersAuth := r.Group("/users", authorization.JWTAuth(cfg.JWT))
	{
		usersAuth.POST("/:id/follow", followUserH.FollowUser)
		usersAuth.DELETE("/:id/follow", followUserH.UnfollowUser)
	}
}
