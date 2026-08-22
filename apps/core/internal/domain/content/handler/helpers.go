package handler

import (
	"encoding/json"
	"strconv"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/content/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type helper struct {
	db *gorm.DB
}

func newHelper() helper {
	return helper{db: database.DB}
}

func (h helper) getLikedIDs(c *gin.Context, targetType string) map[int64]bool {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		return map[int64]bool{}
	}
	var likes []model.Like
	if err := h.db.WithContext(c.Request.Context()).Where("user_id = ? AND target_type = ?", userID, targetType).Find(&likes).Error; err != nil {
		return map[int64]bool{}
	}
	result := make(map[int64]bool, len(likes))
	for _, like := range likes {
		result[like.TargetID] = true
	}
	return result
}

func (h helper) loadMerchantBrief(id int64) *dto.MerchantBrief {
	var merchant model.Merchant
	if err := h.db.First(&merchant, id).Error; err != nil {
		return nil
	}
	return &dto.MerchantBrief{ID: merchant.ID, Name: merchant.Name, Category: merchant.Category}
}

func parseIDParam(c *gin.Context, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

func parseCursorLimit(c *gin.Context) (*int64, int) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	var cursor *int64
	if v := c.Query("cursor"); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cursor = &parsed
		}
	}
	return cursor, limit
}

func parseJSONStrings(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return values
}
