package visibility

import (
	"context"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestCanViewUserContentHonorsPrivacyAndFollowRelationship(t *testing.T) {
	db := testutil.SetupTestDB(t)
	users := []model.User{{Role: "user"}, {Role: "user"}, {Role: "user"}}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	owner, follower, unrelated := users[0], users[1], users[2]
	if err := db.Create(&model.UserPrivacy{UserID: owner.ID}).Error; err != nil {
		t.Fatalf("create privacy: %v", err)
	}
	if err := db.Model(&model.UserPrivacy{}).Where("user_id = ?", owner.ID).Update("is_public", false).Error; err != nil {
		t.Fatalf("make privacy private: %v", err)
	}
	if err := db.Create(&model.UserFollow{FollowerID: follower.ID, FollowingID: owner.ID}).Error; err != nil {
		t.Fatalf("create follow: %v", err)
	}

	cases := []struct {
		name   string
		viewer int64
		want   bool
	}{
		{name: "anonymous denied", viewer: 0, want: false},
		{name: "owner allowed", viewer: owner.ID, want: true},
		{name: "follower allowed", viewer: follower.ID, want: true},
		{name: "unrelated denied", viewer: unrelated.ID, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanViewUserContent(context.Background(), db, owner.ID, tc.viewer)
			if err != nil {
				t.Fatalf("visibility check: %v", err)
			}
			if got != tc.want {
				t.Fatalf("visibility = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestScopePublicContentFiltersPrivateOwners(t *testing.T) {
	db := testutil.SetupTestDB(t)
	users := []model.User{{Role: "user"}, {Role: "user"}}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	publicOwner, privateOwner := users[0], users[1]
	if err := db.Create(&model.UserPrivacy{UserID: privateOwner.ID}).Error; err != nil {
		t.Fatalf("create privacy: %v", err)
	}
	if err := db.Model(&model.UserPrivacy{}).Where("user_id = ?", privateOwner.ID).Update("is_public", false).Error; err != nil {
		t.Fatalf("make privacy private: %v", err)
	}
	if err := db.Create(&[]model.Post{
		{UserID: publicOwner.ID, Content: "public"},
		{UserID: privateOwner.ID, Content: "private"},
	}).Error; err != nil {
		t.Fatalf("create posts: %v", err)
	}

	var posts []model.Post
	if err := ScopePublicContent(db.Model(&model.Post{}), "posts.user_id", 0).Find(&posts).Error; err != nil {
		t.Fatalf("query visible posts: %v", err)
	}
	if len(posts) != 1 || posts[0].UserID != publicOwner.ID {
		t.Fatalf("visible posts = %#v, want only public owner %d", posts, publicOwner.ID)
	}
}
