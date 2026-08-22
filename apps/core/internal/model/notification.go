package model

import "time"

const (
	NotificationTypeOrderPaid                   = "order_paid"
	NotificationTypeVoucherRedeemed             = "voucher_redeemed"
	NotificationTypeMerchantVerificationChanged = "merchant_verification_status_changed"
	NotificationTypeReviewLiked                 = "review_liked"
	NotificationTypeReviewCommented             = "review_commented"
)

type Notification struct {
	ID        int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64      `gorm:"not null;index" json:"user_id"`
	Type      string     `gorm:"type:varchar(50);not null" json:"type"`
	Title     string     `gorm:"type:varchar(255)" json:"title"`
	Content   string     `gorm:"type:text" json:"content"`
	Data      string     `gorm:"type:jsonb;default:'{}'" json:"data"`
	DedupKey  *string    `gorm:"type:varchar(255);index" json:"-"`
	IsRead    bool       `gorm:"default:false" json:"is_read"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (n *Notification) TableName() string { return "notifications" }
