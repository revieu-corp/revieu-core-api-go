package model

import (
	"time"

	"gorm.io/gorm"
)

type Coupon struct {
	ID                 int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	MerchantID         int64          `gorm:"not null;index" json:"merchant_id"`
	StoreID            *int64         `gorm:"index" json:"store_id"`
	PackageID          *int64         `gorm:"index" json:"package_id"`
	Title              string         `gorm:"type:varchar(100);not null" json:"title"`
	Description        string         `gorm:"type:text" json:"description"`
	ImageURL           string         `gorm:"type:varchar(255)" json:"image_url"`
	Type               string         `gorm:"type:varchar(20);not null" json:"type"`
	CouponType         string         `gorm:"type:varchar(20)" json:"coupon_type"`
	Value              string         `gorm:"type:varchar(50)" json:"value"`
	Price              float64        `json:"price"`
	OriginalPrice      float64        `gorm:"type:numeric(10,2)" json:"original_price"`
	SalePrice          float64        `gorm:"type:numeric(10,2)" json:"sale_price"`
	DiscountPercentage float64        `gorm:"type:numeric(5,2)" json:"discount_percentage"`
	DishIDs            string         `gorm:"type:jsonb;default:'[]'" json:"dish_ids"`
	TotalQuantity      int            `gorm:"default:0" json:"total_quantity"`
	ClaimedCount       int            `gorm:"default:0" json:"claimed_count"`
	RedeemedCount      int            `gorm:"default:0" json:"redeemed_count"`
	MaxPerUser         int            `gorm:"default:1" json:"max_per_user"`
	Terms              string         `gorm:"type:text" json:"terms"`
	ExpiryDate         time.Time      `json:"expiry_date"`
	ValidFrom          *time.Time     `json:"valid_from"`
	ValidUntil         *time.Time     `json:"valid_until"`
	Status             string         `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (c *Coupon) TableName() string { return "coupons" }
