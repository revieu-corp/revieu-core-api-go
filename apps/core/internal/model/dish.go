package model

import (
	"time"

	"gorm.io/gorm"
)

// Dish is a merchant-owned menu item. StoreID is optional because the current
// merchant UI manages a merchant-wide menu and can later scope items to a
// particular store without changing the API shape.
type Dish struct {
	ID            int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	MerchantID    int64          `gorm:"not null;index" json:"merchant_id"`
	StoreID       *int64         `gorm:"index" json:"store_id,omitempty"`
	Name          string         `gorm:"type:varchar(255);not null" json:"name"`
	ImageURL      string         `gorm:"type:varchar(512)" json:"image_url"`
	Description   string         `gorm:"type:text" json:"description"`
	OriginalPrice float64        `gorm:"type:numeric(10,2)" json:"original_price"`
	Category      string         `gorm:"type:varchar(100)" json:"category"`
	Status        string         `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (d *Dish) TableName() string { return "dishes" }
