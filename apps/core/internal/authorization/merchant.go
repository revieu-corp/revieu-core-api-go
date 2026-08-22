package authorization

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

const (
	MerchantIDKey                 = "merchant_id"
	MerchantVerificationStatusKey = "merchant_verification_status"
)

// MerchantAccount requires an active merchant record for the authenticated
// user. Draft onboarding is intentionally allowed to create the merchant
// record; publish-grade operations should use VerifiedMerchant instead.
func MerchantAccount() gin.HandlerFunc {
	return merchantEligibility(false)
}

// VerifiedMerchant requires an active merchant whose verification review has
// completed successfully. This is the single HTTP gate for publish-grade
// merchant actions such as activating stores, managing purchasable coupons,
// managing dishes, and redeeming customer vouchers.
func VerifiedMerchant() gin.HandlerFunc {
	return merchantEligibility(true)
}

func merchantEligibility(requireVerified bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64(UserIDKey)
		if userID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if database.DB == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "database unavailable"})
			return
		}

		var merchant model.Merchant
		err := database.DB.WithContext(c.Request.Context()).Where("user_id = ?", userID).First(&merchant).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "merchant account required"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to load merchant account"})
			return
		}
		if merchant.Status != 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "merchant account inactive"})
			return
		}

		verificationStatus := strings.ToLower(strings.TrimSpace(merchant.VerificationStatus))
		if requireVerified && verificationStatus != "verified" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "merchant verification required"})
			return
		}

		c.Set(MerchantIDKey, merchant.ID)
		c.Set(MerchantVerificationStatusKey, verificationStatus)
		c.Next()
	}
}
