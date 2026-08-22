package service

import (
	"context"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestHomeReturnsDeterministicMixedFeedPages(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewFeedService(db)
	user := model.User{Role: "user", Status: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	merchantTimes := []time.Time{
		time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC),
	}
	for i, createdAt := range merchantTimes {
		merchant := model.Merchant{
			Name:               []string{"Featured Cafe", "Featured Deli"}[i],
			VerificationStatus: "verified",
			Status:             0,
			CreatedAt:          createdAt,
			UpdatedAt:          createdAt,
		}
		if err := db.Create(&merchant).Error; err != nil {
			t.Fatalf("create merchant: %v", err)
		}
	}
	for i, createdAt := range []time.Time{
		time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 22, 7, 0, 0, 0, time.UTC),
	} {
		review := model.Review{
			UserID:     user.ID,
			VenueID:    int64(i + 1),
			MerchantID: int64(i + 1),
			Rating:     4,
			Content:    []string{"Great coffee", "Fresh sandwiches"}[i],
			Images:     `[]`,
			VisitDate:  createdAt,
			CreatedAt:  createdAt,
			UpdatedAt:  createdAt,
		}
		if err := db.Create(&review).Error; err != nil {
			t.Fatalf("create review: %v", err)
		}
	}

	first, cursor, err := svc.Home(context.Background(), 0, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 2 || cursor == nil {
		t.Fatalf("first page = %#v cursor=%v, want two items and cursor", first, cursor)
	}
	if first[0].ID != "merchant:1" || first[1].ID != "review:1" {
		t.Fatalf("unexpected first page order: %#v", first)
	}

	second, nextCursor, err := svc.Home(context.Background(), 0, *cursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 2 || nextCursor != nil {
		t.Fatalf("second page = %#v cursor=%v, want two items and no cursor", second, nextCursor)
	}
	if second[0].ID == first[0].ID || second[0].ID == first[1].ID || second[1].ID == first[0].ID || second[1].ID == first[1].ID {
		t.Fatalf("feed pagination repeated an item: first=%#v second=%#v", first, second)
	}

}

func TestHomeRejectsMalformedCursor(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewFeedService(db)
	if _, _, err := svc.Home(context.Background(), 0, "not-a-cursor", 20); err != ErrInvalidCursor {
		t.Fatalf("error = %v, want %v", err, ErrInvalidCursor)
	}
}

func TestHomeIncludesFollowedPrivateAuthorContentForViewer(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewFeedService(db)
	viewer := model.User{Role: "user", Status: 0}
	author := model.User{Role: "user", Status: 0}
	if err := db.Create(&[]model.User{viewer, author}).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	if err := db.Create(&model.UserPrivacy{UserID: author.ID}).Error; err != nil {
		t.Fatalf("create privacy: %v", err)
	}
	if err := db.Model(&model.UserPrivacy{}).Where("user_id = ?", author.ID).Update("is_public", false).Error; err != nil {
		t.Fatalf("make author private: %v", err)
	}
	if err := db.Create(&model.UserFollow{FollowerID: viewer.ID, FollowingID: author.ID}).Error; err != nil {
		t.Fatalf("create follow: %v", err)
	}
	if err := db.Create(&model.Post{UserID: author.ID, Content: "Follow-only update"}).Error; err != nil {
		t.Fatalf("create post: %v", err)
	}

	items, _, err := svc.Home(context.Background(), viewer.ID, "", 20)
	if err != nil {
		t.Fatalf("home feed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "post:1" {
		t.Fatalf("followed private post not returned: %#v", items)
	}
}
