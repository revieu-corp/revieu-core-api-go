package auth

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/token"
)

type passwordResetMailer struct {
	to       string
	resetURL string
	calls    int
}

func (m *passwordResetMailer) SendVerificationEmail(string, string) error {
	return nil
}

func (m *passwordResetMailer) SendPasswordResetEmail(to, resetURL string) error {
	m.to = to
	m.resetURL = resetURL
	m.calls++
	return nil
}

func newPasswordResetTestService(t *testing.T) (*service, *model.User, *passwordResetMailer) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	user := &model.User{Role: "user", Status: 0}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	auth := model.UserAuth{UserID: user.ID, IdentityType: "email", Identifier: "reset@example.com"}
	if err := auth.SetPassword("old-password"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if err := db.Create(&auth).Error; err != nil {
		t.Fatalf("create auth: %v", err)
	}

	svc := NewService(db, testJWTConfig, testSMTPConfig).(*service)
	mailer := &passwordResetMailer{}
	svc.emailClient = mailer
	return svc, user, mailer
}

func TestRequestAndConsumePasswordResetToken(t *testing.T) {
	svc, user, mailer := newPasswordResetTestService(t)
	ctx := context.Background()

	if err := svc.RequestPasswordReset(ctx, "reset@example.com", "https://review.example"); err != nil {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}
	if mailer.calls != 1 || mailer.to != "reset@example.com" {
		t.Fatalf("expected one reset email, got %+v", mailer)
	}
	parsed, err := url.Parse(mailer.resetURL)
	if err != nil {
		t.Fatalf("parse reset URL: %v", err)
	}
	rawToken := parsed.Query().Get("token")
	if rawToken == "" || strings.Contains(mailer.resetURL, token.HashToken(rawToken)) {
		t.Fatal("reset URL should contain the raw token, not its hash")
	}

	refresh := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: strings.Repeat("a", 64),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := svc.db.Create(&refresh).Error; err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	if err := svc.ResetPassword(ctx, rawToken, "new-password"); err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}
	var auth model.UserAuth
	if err := svc.db.Where("user_id = ? AND identity_type = ?", user.ID, "email").First(&auth).Error; err != nil {
		t.Fatalf("load auth: %v", err)
	}
	if !auth.CheckPassword("new-password") || auth.CheckPassword("old-password") {
		t.Fatal("expected password to be replaced")
	}
	var refreshed model.RefreshToken
	if err := svc.db.First(&refreshed, refresh.ID).Error; err != nil {
		t.Fatalf("load refresh token: %v", err)
	}
	if refreshed.RevokedAt == nil {
		t.Fatal("expected existing refresh session to be revoked")
	}
	if err := svc.ResetPassword(ctx, rawToken, "another-password"); err == nil {
		t.Fatal("expected reset token reuse to fail")
	}

	var stored model.PasswordResetToken
	if err := svc.db.Where("user_id = ?", user.ID).First(&stored).Error; err != nil {
		t.Fatalf("load reset token: %v", err)
	}
	if stored.TokenHash == rawToken || stored.UsedAt == nil {
		t.Fatal("expected hashed and consumed reset token")
	}
}

func TestPasswordResetUnknownAccountIsNonEnumerating(t *testing.T) {
	svc, _, mailer := newPasswordResetTestService(t)
	if err := svc.RequestPasswordReset(context.Background(), "unknown@example.com", "https://review.example"); err != nil {
		t.Fatalf("unknown account should not return an error: %v", err)
	}
	if mailer.calls != 0 {
		t.Fatalf("unknown account should not receive email, got %d calls", mailer.calls)
	}
}

func TestPasswordResetRateLimit(t *testing.T) {
	svc, user, mailer := newPasswordResetTestService(t)
	for i := 0; i < passwordResetMaxRequests+1; i++ {
		if err := svc.RequestPasswordReset(context.Background(), "reset@example.com", "https://review.example"); err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
	}
	if mailer.calls != passwordResetMaxRequests {
		t.Fatalf("expected %d reset emails, got %d", passwordResetMaxRequests, mailer.calls)
	}
	var count int64
	if err := svc.db.Model(&model.PasswordResetToken{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count reset tokens: %v", err)
	}
	if count != passwordResetMaxRequests {
		t.Fatalf("expected %d persisted reset tokens, got %d", passwordResetMaxRequests, count)
	}
}

func TestPasswordResetRejectsExpiredToken(t *testing.T) {
	svc, user, _ := newPasswordResetTestService(t)
	if err := svc.db.Create(&model.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: token.HashToken("expired-token"),
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
	}).Error; err != nil {
		t.Fatalf("create expired token: %v", err)
	}
	if err := svc.ResetPassword(context.Background(), "expired-token", "new-password"); err == nil {
		t.Fatal("expected expired reset token to fail")
	}
}
