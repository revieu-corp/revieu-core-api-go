package dto

import "time"

type ReviewItem struct {
	ID               int64         `json:"id"`
	Rating           float32       `json:"rating"`
	RatingEnv        *float32      `json:"rating_env,omitempty"`
	RatingService    *float32      `json:"rating_service,omitempty"`
	RatingValue      *float32      `json:"rating_value,omitempty"`
	Content          string        `json:"content"`
	Images           []string      `json:"images"`
	AvgCost          *int          `json:"avg_cost,omitempty"`
	LikeCount        int           `json:"like_count"`
	CommentCount     int           `json:"comment_count"`
	LocationVerified bool          `json:"location_verified"`
	IsLiked          bool          `json:"is_liked"`
	Merchant         MerchantBrief `json:"merchant"`
	Tags             []string      `json:"tags"`
	CreatedAt        time.Time     `json:"created_at"`
}

type ReviewListResponse struct {
	Reviews []ReviewItem `json:"reviews"`
	Total   int          `json:"total"`
	Cursor  *int64       `json:"cursor,omitempty"`
}
