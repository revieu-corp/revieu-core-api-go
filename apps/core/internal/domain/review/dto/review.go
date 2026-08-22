package dto

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
)

type Review struct {
	ID               string   `json:"id"`
	MerchantID       string   `json:"merchantId"`
	VenueID          string   `json:"venueId"`
	StoreID          string   `json:"storeId"`
	UserID           string   `json:"userId"`
	Rating           float64  `json:"rating"`
	RatingEnv        *float64 `json:"ratingEnv,omitempty"`
	RatingService    *float64 `json:"ratingService,omitempty"`
	RatingValue      *float64 `json:"ratingValue,omitempty"`
	RatingFood       *float64 `json:"ratingFood,omitempty"`
	LocationVerified bool     `json:"locationVerified"`
	Text             string   `json:"text"`
	Images           []string `json:"images"`
	Tags             []string `json:"tags"`
	VisitDate        string   `json:"visitDate"`
	CreatedAt        string   `json:"createdAt"`
	BusinessName     string   `json:"businessName"`
	BusinessImage    string   `json:"businessImage"`
	Location         string   `json:"location"`
	LikeCount        int      `json:"likeCount"`
}

// CommentRequest is the request body for adding a review comment.
type CommentRequest struct {
	Text string `json:"text" binding:"required"`
}

func (r Review) MerchantIDValue() (int64, error) {
	if r.MerchantID == "" {
		return 0, errors.New("merchantId required")
	}
	return strconv.ParseInt(r.MerchantID, 10, 64)
}

func (r Review) StoreIDValue() (*int64, error) {
	if r.StoreID == "" {
		return nil, nil
	}
	v, err := strconv.ParseInt(r.StoreID, 10, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r Review) VenueIDValue() (int64, error) {
	if r.VenueID == "" {
		return 0, errors.New("venueId required")
	}
	return strconv.ParseInt(r.VenueID, 10, 64)
}

func (r Review) VisitDateValue() (time.Time, error) {
	if r.VisitDate == "" {
		return time.Now(), nil
	}
	return time.Parse("2006-01-02", r.VisitDate)
}

func FromModel(m model.Review) Review {
	var images []string
	if m.Images != "" {
		_ = json.Unmarshal([]byte(m.Images), &images)
	}
	if images == nil {
		images = []string{}
	}

	var businessName, businessImage, location string
	if m.Merchant != nil {
		businessName = m.Merchant.Name
		if m.Merchant.BusinessName != "" {
			businessName = m.Merchant.BusinessName
		}
		businessImage = m.Merchant.CoverImage
		location = m.Merchant.Address
	}

	var storeID string
	if m.StoreID != nil {
		storeID = strconv.FormatInt(*m.StoreID, 10)
	}

	return Review{
		ID:               strconv.FormatInt(m.ID, 10),
		MerchantID:       strconv.FormatInt(m.MerchantID, 10),
		VenueID:          strconv.FormatInt(m.VenueID, 10),
		StoreID:          storeID,
		UserID:           strconv.FormatInt(m.UserID, 10),
		Rating:           float64(m.Rating),
		RatingEnv:        float32ToFloat64(m.RatingEnv),
		RatingService:    float32ToFloat64(m.RatingService),
		RatingValue:      float32ToFloat64(m.RatingValue),
		RatingFood:       float32ToFloat64(m.RatingFood),
		LocationVerified: m.LocationVerified,
		Text:             m.Content,
		Images:           images,
		Tags:             tagNames(m.Tags),
		VisitDate:        m.VisitDate.Format("2006-01-02"),
		CreatedAt:        m.CreatedAt.Format(time.RFC3339),
		BusinessName:     businessName,
		BusinessImage:    businessImage,
		Location:         location,
		LikeCount:        m.LikeCount,
	}
}

func float32ToFloat64(value *float32) *float64 {
	if value == nil {
		return nil
	}
	converted := float64(*value)
	return &converted
}

func tagNames(tags []model.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return names
}

func FromModels(items []model.Review) []Review {
	out := make([]Review, 0, len(items))
	for _, item := range items {
		out = append(out, FromModel(item))
	}
	return out
}
