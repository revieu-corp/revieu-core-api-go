package model

import "time"

type Review struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID           int64     `gorm:"not null;index" json:"user_id"`
	VenueID          int64     `gorm:"not null;index" json:"venue_id"`
	MerchantID       int64     `gorm:"not null;index" json:"merchant_id"`
	StoreID          *int64    `gorm:"index" json:"store_id"`
	Rating           float32   `gorm:"not null" json:"rating"`
	RatingEnv        *float32  `json:"rating_env"`
	RatingService    *float32  `json:"rating_service"`
	RatingValue      *float32  `json:"rating_value"`
	RatingFood       *float32  `json:"rating_food"`
	Content          string    `gorm:"type:text" json:"content"`
	Images           string    `gorm:"type:jsonb;default:'[]'" json:"images"`
	VisitDate        time.Time `gorm:"not null;type:date" json:"visit_date"`
	AvgCost          *int      `json:"avg_cost"`
	LikeCount        int       `gorm:"default:0" json:"like_count"`
	CommentCount     int       `gorm:"default:0" json:"comment_count"`
	LocationVerified bool      `gorm:"not null;default:false" json:"location_verified"`
	UserNickname     string    `gorm:"-" json:"user_nickname,omitempty"`
	UserAvatar       string    `gorm:"-" json:"user_avatar,omitempty"`
	StoreName        string    `gorm:"-" json:"store_name,omitempty"`
	Status           int16     `gorm:"default:0" json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	User     *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Merchant *Merchant `gorm:"foreignKey:MerchantID" json:"merchant,omitempty"`
	Store    *Store    `gorm:"foreignKey:StoreID" json:"store,omitempty"`
	Tags     []Tag     `gorm:"many2many:review_tags" json:"tags,omitempty"`
}

func (r *Review) TableName() string {
	return "reviews"
}
