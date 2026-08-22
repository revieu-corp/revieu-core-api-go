package testutil

import (
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SetupTestDB creates an in-memory sqlite DB with schema migrations.
func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: nil})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAuth{},
		&model.UserProfile{},
		&model.EmailVerification{},
		&model.RefreshToken{},
		&model.Merchant{},
		&model.Store{},
		&model.StoreHour{},
		&model.Category{},
		&model.StoreCategory{},
		&model.Tag{},
		&model.Post{},
		&model.Review{},
		&model.ReviewComment{},
		&model.Package{},
		&model.Coupon{},
		&model.Order{},
		&model.Voucher{},
		&model.Payment{},
		&model.MediaUpload{},
		&model.UserFollow{},
		&model.MerchantFollow{},
		&model.Like{},
		&model.Favorite{},
		&model.UserAddress{},
		&model.UserPrivacy{},
		&model.UserNotification{},
		&model.AccountDeletion{},
		&model.Conversation{},
		&model.ConversationParticipant{},
		&model.Message{},
		&model.MerchantVerification{},
	); err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}
