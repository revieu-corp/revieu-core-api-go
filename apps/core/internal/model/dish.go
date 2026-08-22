package model

import (
	"time"

	"gorm.io/gorm"
)

// Dish is a menu item owned by a merchant. It is merchant-private in this
// pass (no public read endpoint) — its ImageURL is copied onto a Coupon's
// ImageURL when a coupon is created against it.
type Dish struct {
	ID            int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	MerchantID    int64          `gorm:"not null;index" json:"merchant_id"`
	Name          string         `gorm:"type:varchar(100);not null" json:"name"`
	ImageURL      string         `gorm:"type:varchar(255)" json:"image_url"`
	Description   string         `gorm:"type:text" json:"description"`
	OriginalPrice float64        `gorm:"type:numeric(10,2);not null;default:0" json:"original_price"`
	Category      string         `gorm:"type:varchar(50)" json:"category"`
	Status        string         `gorm:"type:varchar(20);not null;default:'active'" json:"status"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (d *Dish) TableName() string {
	return "dishes"
}
