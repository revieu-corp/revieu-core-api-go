package voucher

import (
	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/voucher/handler"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/voucher/service"
)

// RegisterRoutes registers voucher routes.
func RegisterRoutes(r *gin.RouterGroup, cfg *config.Config) {
	svc := service.NewVoucherService(nil)
	h := handler.NewVoucherHandler(svc, cfg.FrontendURL)

	vouchers := r.Group("/vouchers", authorization.JWTAuth(cfg.JWT))
	{
		vouchers.POST("", h.Create)
		vouchers.GET("", h.List)
		vouchers.GET("/:id", h.Detail)
		vouchers.GET("/code/:code", h.ByCode)
		vouchers.PATCH("/:id/use", h.Use)
		vouchers.PATCH("/:id/status", h.UpdateStatus)
		vouchers.POST("/share/email", h.ShareEmail)
		vouchers.POST("/share/sms", h.ShareSMS)
	}

	merchantVouchers := r.Group("/merchant/vouchers", authorization.JWTAuth(cfg.JWT))
	{
		merchantVouchers.GET("/scan", authorization.VerifiedMerchant(), h.ScanPreview)
		merchantVouchers.POST("/redeem-by-token", authorization.VerifiedMerchant(), h.RedeemByToken)
		merchantVouchers.POST("/:id/redeem", authorization.VerifiedMerchant(), h.RedeemByMerchant)
	}
}
