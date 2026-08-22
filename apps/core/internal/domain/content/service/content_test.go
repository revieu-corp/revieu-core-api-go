package service

import (
	"context"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestContentServiceListPosts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewContentService(db)
	user := model.User{Role: "user", Status: 0}
	db.Create(&user)
	for _, content := range []string{"a", "b", "c"} {
		db.Create(&model.Post{UserID: user.ID, Content: content})
	}
	posts, total, cursor, err := svc.ListUserPosts(context.Background(), user.ID, nil, 2)
	if err != nil || total != 3 || len(posts) != 2 || cursor == nil {
		t.Fatalf("list first posts page failed: err=%v total=%d len=%d cursor=%v", err, total, len(posts), cursor)
	}
	second, secondTotal, secondCursor, err := svc.ListUserPosts(context.Background(), user.ID, cursor, 2)
	if err != nil || secondTotal != 3 || len(second) != 1 || secondCursor != nil {
		t.Fatalf("list second posts page failed: err=%v total=%d len=%d cursor=%v", err, secondTotal, len(second), secondCursor)
	}
	if posts[0].ID == second[0].ID || posts[1].ID == second[0].ID {
		t.Fatalf("post pagination repeated id %d", second[0].ID)
	}
}

func TestContentServiceListFavoritesAndLikesPaginate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewContentService(db)
	user := model.User{Role: "user", Status: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	for i := int64(1); i <= 3; i++ {
		if err := db.Create(&model.Favorite{UserID: user.ID, TargetType: "post", TargetID: i}).Error; err != nil {
			t.Fatalf("create favorite: %v", err)
		}
		if err := db.Create(&model.Like{UserID: user.ID, TargetType: "post", TargetID: i}).Error; err != nil {
			t.Fatalf("create like: %v", err)
		}
	}

	favorites, total, cursor, err := svc.ListFavorites(context.Background(), user.ID, "post", nil, 2)
	if err != nil || total != 3 || len(favorites) != 2 || cursor == nil {
		t.Fatalf("list favorites page failed: err=%v total=%d len=%d cursor=%v", err, total, len(favorites), cursor)
	}
	secondFavorites, _, secondCursor, err := svc.ListFavorites(context.Background(), user.ID, "post", cursor, 2)
	if err != nil || len(secondFavorites) != 1 || secondCursor != nil {
		t.Fatalf("list second favorites page failed: err=%v len=%d cursor=%v", err, len(secondFavorites), secondCursor)
	}

	likes, total, cursor, err := svc.ListLikes(context.Background(), user.ID, nil, 2)
	if err != nil || total != 3 || len(likes) != 2 || cursor == nil {
		t.Fatalf("list likes page failed: err=%v total=%d len=%d cursor=%v", err, total, len(likes), cursor)
	}
	secondLikes, _, secondCursor, err := svc.ListLikes(context.Background(), user.ID, cursor, 2)
	if err != nil || len(secondLikes) != 1 || secondCursor != nil {
		t.Fatalf("list second likes page failed: err=%v len=%d cursor=%v", err, len(secondLikes), secondCursor)
	}
}
