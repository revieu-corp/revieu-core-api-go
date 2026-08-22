package stores

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	couponHandler "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/coupon/handler"
	couponService "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/coupon/service"
	storeHandler "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/store/handler"
	storeService "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/store/service"
)

// RegisterRoutes consolidates all /stores and /merchant/stores routes from store
// and coupon domains. Single owner for the stores resource prefix.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	// Store handlers
	storeSvc := storeService.NewStoreService(nil)
	storeH := storeHandler.NewStoreHandler(storeSvc)

	// Coupon handlers
	couponSvc := couponService.NewCouponService(nil)
	couponH := couponHandler.NewCouponHandler(couponSvc)

	// Public: store list, detail, reviews, hours, and coupons
	storesPublic := r.Group("/stores")
	{
		storesPublic.GET("", storeH.List)
		storesPublic.GET("/:id", storeH.Detail)
		storesPublic.GET("/:id/reviews", storeH.Reviews)
		storesPublic.GET("/:id/hours", storeH.Hours)
		storesPublic.GET("/:id/coupons", couponH.ListStoreCoupons)
	}

	// Authenticated: merchant store and coupon management
	merchantStoresAuth := r.Group("/merchant/stores", authorization.JWTAuth(cfg.JWT))
	{
		merchantStoresAuth.GET("", storeH.ListMine)
		merchantStoresAuth.POST("", storeH.Create)
		merchantStoresAuth.POST("/:id/activate", storeH.Activate)
		merchantStoresAuth.POST("/:id/deactivate", storeH.Deactivate)
		merchantStoresAuth.PATCH("/:id", storeH.Update)
		merchantStoresAuth.DELETE("/:id", storeH.Delete)
		merchantStoresAuth.GET("/:id/coupons", couponH.ListMerchantStoreCoupons)
		merchantStoresAuth.POST("/:id/coupons", couponH.CreateStoreCoupon)
		merchantStoresAuth.PATCH("/:id/coupons/:couponId", couponH.UpdateStoreCoupon)
		merchantStoresAuth.POST("/:id/coupons/:couponId/enable", couponH.EnableStoreCoupon)
		merchantStoresAuth.POST("/:id/coupons/:couponId/disable", couponH.DisableStoreCoupon)
		merchantStoresAuth.DELETE("/:id/coupons/:couponId", couponH.DeleteStoreCoupon)
	}

	// Authenticated: merchant-wide coupon lifecycle management. The service
	// enforces the merchant principal and verified gate for activation.
	merchantCouponsAuth := r.Group("/merchant/coupons", authorization.JWTAuth(cfg.JWT))
	{
		merchantCouponsAuth.GET("", couponH.ListMerchantCoupons)
		merchantCouponsAuth.PATCH("/:id", couponH.UpdateMerchantCoupon)
		merchantCouponsAuth.POST("/:id/activate", couponH.ActivateMerchantCoupon)
		merchantCouponsAuth.POST("/:id/deactivate", couponH.DeactivateMerchantCoupon)
	}
}
