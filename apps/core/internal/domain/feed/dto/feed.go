package dto

import "time"

type FeedItem struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Image     string    `json:"image"`
	CreatedAt time.Time `json:"created_at"`
}

type FeedResponse struct {
	Data   []FeedItem `json:"data"`
	Cursor *string    `json:"cursor,omitempty"`
}
