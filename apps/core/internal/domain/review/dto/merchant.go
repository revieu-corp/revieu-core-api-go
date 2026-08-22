package dto

// MerchantReview is the dashboard-facing representation of a review owned by
// the authenticated merchant. It deliberately exposes only the fields the
// merchant review workflow needs.
type MerchantReview struct {
	ID           int64   `json:"id"`
	CustomerName string  `json:"customerName"`
	Rating       float32 `json:"rating"`
	Text         string  `json:"text"`
	Date         string  `json:"date"`
	HasReply     bool    `json:"hasReply"`
	ReplyText    string  `json:"replyText,omitempty"`
}

type MerchantReviewListResponse struct {
	Data []MerchantReview `json:"data"`
}

type MerchantReplyRequest struct {
	Text string `json:"text" binding:"required,max=500"`
}
