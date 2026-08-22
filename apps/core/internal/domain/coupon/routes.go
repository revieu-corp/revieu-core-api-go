package coupon

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/coupon/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/coupon/service"
)

// RegisterRoutes registers coupon and package routes. Store-scoped coupon
// routes live in the stores domain, which owns the /stores prefix.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewCouponService(nil)
	h := handler.NewCouponHandler(svc)

	// Public: validate and initiate payment
	couponsPublic := r.Group("/coupons")
	{
		couponsPublic.GET("/:id", h.GetPublishedCoupon)
		couponsPublic.POST("/:id/validate", h.Validate)
		couponsPublic.POST("/:id/payment/initiate", h.InitiatePayment)
	}

	// Authenticated: redeem coupons
	couponsAuth := r.Group("/coupons", authorization.JWTAuth(cfg.JWT))
	{
		couponsAuth.POST("/:id/redeem", h.Redeem)
	}

	// Public: packages
	packages := r.Group("/packages")
	{
		packages.GET("", h.ListPackages)
		packages.GET("/:id", h.PackageDetail)
	}
}
