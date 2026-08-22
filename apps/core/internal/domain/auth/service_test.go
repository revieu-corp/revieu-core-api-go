package auth

import (
	"context"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/token"
)

var testJWTConfig = config.JWTConfig{
	Secret:     "test-secret-key-for-testing",
	ExpireHour: 24,
}

var testSMTPConfig = config.SMTPConfig{
	Host:     "",
	Port:     0,
	Username: "",
	Password: "",
	From:     "test@example.com",
	UseTLS:   false,
}

func TestRegister(t *testing.T) {
	db := testutil.SetupTestDB(t)
	authService := NewService(db, testJWTConfig, testSMTPConfig)

	ctx := context.Background()
	username := "testuser"
	email := "test@example.com"
	password := "password123"
	baseURL := "http://localhost:8080"

	user, err := authService.Register(ctx, username, email, password, baseURL)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}
	if user.ID == 0 {
		t.Error("Expected user ID to be generated")
	}
	if user.Role != "user" {
		t.Errorf("Expected role 'user', got %s", user.Role)
	}
	if user.Status != 2 {
		t.Errorf("Expected status 2 (pending), got %d", user.Status)
	}

	var verification model.EmailVerification
	if err := db.Where("user_id = ?", user.ID).First(&verification).Error; err != nil {
		t.Errorf("Expected email verification record to be created: %v", err)
	}

	_, err = authService.Register(ctx, "otheruser", email, "pass", baseURL)
	if err == nil {
		t.Error("Expected error for duplicate email, got nil")
	}
}

func TestLogin(t *testing.T) {
	db := testutil.SetupTestDB(t)
	authService := NewService(db, testJWTConfig, testSMTPConfig)

	ctx := context.Background()
	email := "login@example.com"
	password := "securepass"
	username := "loginuser"
	baseURL := "http://localhost"

	user, err := authService.Register(ctx, username, email, password, baseURL)
	if err != nil {
		t.Fatalf("Failed to create user for login test: %v", err)
	}

	_, err = authService.Login(ctx, email, password, "127.0.0.1")
	if err == nil {
		t.Error("Expected error for unverified user, got nil")
	}

	var verification model.EmailVerification
	if err := db.Where("user_id = ?", user.ID).First(&verification).Error; err != nil {
		t.Fatalf("Failed to find verification record: %v", err)
	}
	if err := authService.VerifyEmail(ctx, verification.Token); err != nil {
		t.Fatalf("Failed to verify email: %v", err)
	}

	tokens, err := authService.Login(ctx, email, password, "127.0.0.1")
	if err != nil {
		t.Errorf("Login failed: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Error("Expected JWT access token, got empty string")
	}
	if tokens.RefreshToken == "" {
		t.Error("Expected refresh token, got empty string")
	}

	_, err = authService.Login(ctx, email, "wrongpass", "127.0.0.1")
	if err == nil {
		t.Error("Expected error for wrong password, got nil")
	}

	_, err = authService.Login(ctx, "nonexistent@example.com", password, "127.0.0.1")
	if err == nil {
		t.Error("Expected error for user not found, got nil")
	}
}

func TestUserProfileHasCounts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	type Column struct{ Name string }
	var cols []Column
	if err := db.Raw("PRAGMA table_info(user_profiles)").Scan(&cols).Error; err != nil {
		t.Fatalf("schema query failed: %v", err)
	}
	want := map[string]bool{
		"follower_count":  true,
		"following_count": true,
		"post_count":      true,
		"review_count":    true,
		"like_count":      true,
	}
	for _, c := range cols {
		delete(want, c.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing columns: %v", want)
	}
}

func TestMerchantAndTagModels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	merchant := model.Merchant{Name: "Cafe", Category: "food"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("merchant create failed: %v", err)
	}
	tag := model.Tag{Name: "#coffee"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("tag create failed: %v", err)
	}
}

func TestPostAndReviewModels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	user := model.User{Role: "user", Status: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	merchant := model.Merchant{Name: "Cafe"}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatal(err)
	}

	post := model.Post{UserID: user.ID, MerchantID: &merchant.ID, Content: "hello"}
	if err := db.Create(&post).Error; err != nil {
		t.Fatal(err)
	}

	review := model.Review{UserID: user.ID, MerchantID: merchant.ID, Rating: 4.5, Content: "great"}
	if err := db.Create(&review).Error; err != nil {
		t.Fatal(err)
	}
}

func TestFollowAndInteractionModels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	u1 := model.User{Role: "user", Status: 0}
	u2 := model.User{Role: "user", Status: 0}
	if err := db.Create(&u1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&u2).Error; err != nil {
		t.Fatal(err)
	}

	follow := model.UserFollow{FollowerID: u1.ID, FollowingID: u2.ID}
	if err := db.Create(&follow).Error; err != nil {
		t.Fatal(err)
	}

	like := model.Like{UserID: u1.ID, TargetType: "post", TargetID: 123}
	if err := db.Create(&like).Error; err != nil {
		t.Fatal(err)
	}
}

func TestSettingsAndAddressModels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	user := model.User{Role: "user", Status: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	privacy := model.UserPrivacy{UserID: user.ID, IsPublic: true}
	if err := db.Create(&privacy).Error; err != nil {
		t.Fatal(err)
	}

	address := model.UserAddress{UserID: user.ID, Name: "A", Phone: "1", Address: "Street"}
	if err := db.Create(&address).Error; err != nil {
		t.Fatal(err)
	}
}

func TestLoginReturnsTokenPairAndPersistsRefreshToken(t *testing.T) {
	db := testutil.SetupTestDB(t)
	authService := NewService(db, testJWTConfig, testSMTPConfig)

	ctx := context.Background()
	email := "pair@example.com"
	password := "securepass"
	username := "pairuser"
	baseURL := "http://localhost"

	user, err := authService.Register(ctx, username, email, password, baseURL)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	var verification model.EmailVerification
	if err := db.Where("user_id = ?", user.ID).First(&verification).Error; err != nil {
		t.Fatalf("Failed to find verification record: %v", err)
	}
	if err := authService.VerifyEmail(ctx, verification.Token); err != nil {
		t.Fatalf("Failed to verify email: %v", err)
	}

	tokens, err := authService.Login(ctx, email, password, "127.0.0.1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if tokens.RefreshToken == "" {
		t.Fatal("expected refresh token")
	}

	var refresh model.RefreshToken
	if err := db.Where("user_id = ? AND revoked_at IS NULL", user.ID).First(&refresh).Error; err != nil {
		t.Fatalf("expected persisted refresh token record: %v", err)
	}
	if refresh.TokenHash != token.HashToken(tokens.RefreshToken) {
		t.Fatal("expected persisted refresh token hash to match returned token")
	}
}

func TestRefreshAccessTokenRotatesRefreshToken(t *testing.T) {
	db := testutil.SetupTestDB(t)
	authService := NewService(db, testJWTConfig, testSMTPConfig)

	ctx := context.Background()
	email := "rotate@example.com"
	password := "securepass"
	username := "rotateuser"
	baseURL := "http://localhost"

	user, err := authService.Register(ctx, username, email, password, baseURL)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	var verification model.EmailVerification
	if err := db.Where("user_id = ?", user.ID).First(&verification).Error; err != nil {
		t.Fatalf("Failed to find verification record: %v", err)
	}
	if err := authService.VerifyEmail(ctx, verification.Token); err != nil {
		t.Fatalf("Failed to verify email: %v", err)
	}

	initialTokens, err := authService.Login(ctx, email, password, "127.0.0.1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	refreshedTokens, err := authService.RefreshAccessToken(ctx, initialTokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshAccessToken failed: %v", err)
	}
	if refreshedTokens.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if refreshedTokens.RefreshToken == "" {
		t.Fatal("expected refresh token")
	}
	if refreshedTokens.RefreshToken == initialTokens.RefreshToken {
		t.Fatal("expected refresh token to rotate")
	}

	var oldRefresh model.RefreshToken
	oldHash := token.HashToken(initialTokens.RefreshToken)
	if err := db.Where("token_hash = ?", oldHash).First(&oldRefresh).Error; err != nil {
		t.Fatalf("expected old refresh token row: %v", err)
	}
	if oldRefresh.RevokedAt == nil {
		t.Fatal("expected old refresh token to be revoked")
	}

	var newRefresh model.RefreshToken
	newHash := token.HashToken(refreshedTokens.RefreshToken)
	if err := db.Where("token_hash = ? AND revoked_at IS NULL", newHash).First(&newRefresh).Error; err != nil {
		t.Fatalf("expected new active refresh token row: %v", err)
	}
}

func TestRefreshAccessTokenRejectsRevokedToken(t *testing.T) {
	db := testutil.SetupTestDB(t)
	authService := NewService(db, testJWTConfig, testSMTPConfig)

	ctx := context.Background()
	email := "revoked@example.com"
	password := "securepass"
	username := "revokeduser"
	baseURL := "http://localhost"

	user, err := authService.Register(ctx, username, email, password, baseURL)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	var verification model.EmailVerification
	if err := db.Where("user_id = ?", user.ID).First(&verification).Error; err != nil {
		t.Fatalf("Failed to find verification record: %v", err)
	}
	if err := authService.VerifyEmail(ctx, verification.Token); err != nil {
		t.Fatalf("Failed to verify email: %v", err)
	}

	initialTokens, err := authService.Login(ctx, email, password, "127.0.0.1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if _, err := authService.RefreshAccessToken(ctx, initialTokens.RefreshToken); err != nil {
		t.Fatalf("first refresh should succeed: %v", err)
	}
	if _, err := authService.RefreshAccessToken(ctx, initialTokens.RefreshToken); err == nil {
		t.Fatal("expected old refresh token to be rejected after rotation")
	}
}

func TestRefreshAccessTokenRejectsInactiveUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	authService := NewService(db, testJWTConfig, testSMTPConfig)

	ctx := context.Background()
	user, err := authService.Register(ctx, "inactive-refresh", "inactive-refresh@example.com", "securepass", "http://localhost")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	var verification model.EmailVerification
	if err := db.Where("user_id = ?", user.ID).First(&verification).Error; err != nil {
		t.Fatalf("find verification: %v", err)
	}
	if err := authService.VerifyEmail(ctx, verification.Token); err != nil {
		t.Fatalf("verify email: %v", err)
	}

	tokens, err := authService.Login(ctx, "inactive-refresh@example.com", "securepass", "127.0.0.1")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("status", 1).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}

	if _, err := authService.RefreshAccessToken(ctx, tokens.RefreshToken); err == nil {
		t.Fatal("expected disabled user refresh to be rejected")
	}
}
