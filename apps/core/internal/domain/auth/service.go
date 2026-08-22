package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/token"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/email"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/logger"
	"gorm.io/gorm"
)

// Service exposes auth operations used by handlers.
type Service interface {
	Register(ctx context.Context, username, userEmail, password, baseURL string) (*model.User, error)
	Login(ctx context.Context, email, password, ipAddress string) (LoginTokens, error)
	RefreshAccessToken(ctx context.Context, refreshToken string) (LoginTokens, error)
	LoginOrRegisterOAuthUser(ctx context.Context, email, name, provider, avatar string) (string, error)
	VerifyEmail(ctx context.Context, token string) error
}

// LoginTokens contains the access and refresh token pair.
type LoginTokens struct {
	AccessToken  string
	RefreshToken string
}

type service struct {
	db           *gorm.DB
	tokenService *token.Service
	emailClient  *email.SMTPClient
}

// NewService creates an auth service.
func NewService(db *gorm.DB, jwtCfg config.JWTConfig, smtpCfg config.SMTPConfig) Service {
	if db == nil {
		db = database.DB
	}
	var emailClient *email.SMTPClient
	if smtpCfg.Host != "" && smtpCfg.Port != 0 {
		emailClient = email.NewSMTPClient(smtpCfg)
	}
	return &service{
		db:           db,
		tokenService: token.New(jwtCfg),
		emailClient:  emailClient,
	}
}

func (s *service) Register(ctx context.Context, username, userEmail, password, baseURL string) (*model.User, error) {
	var existingAuth model.UserAuth
	if err := s.db.Where("identity_type = ? AND identifier = ?", "email", userEmail).First(&existingAuth).Error; err == nil {
		return nil, errors.New("user already exists")
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	token := uuid.New().String()

	var user model.User
	err := s.db.Transaction(func(tx *gorm.DB) error {
		user = model.User{Role: "user", Status: 2}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		auth := model.UserAuth{UserID: user.ID, IdentityType: "email", Identifier: userEmail}
		if err := auth.SetPassword(password); err != nil {
			return err
		}
		if err := tx.Create(&auth).Error; err != nil {
			return err
		}

		profile := model.UserProfile{UserID: user.ID, Nickname: username}
		if err := tx.Create(&profile).Error; err != nil {
			return err
		}

		verification := model.EmailVerification{
			UserID:    user.ID,
			Email:     userEmail,
			Token:     token,
			ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		}
		if err := tx.Create(&verification).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	verifyURL := fmt.Sprintf("%s/auth/verify?token=%s", baseURL, token)

	if s.emailClient == nil {
		logger.Warn(ctx, "SMTP not configured; verification email not sent",
			"event", "user_registered",
			"user_id", user.ID,
			"email", userEmail,
		)
		logger.Info(ctx, fmt.Sprintf("Verification link for %s: %s", userEmail, verifyURL),
			"event", "user_registered",
			"user_id", user.ID,
			"email", userEmail,
		)
	} else if err := s.emailClient.SendVerificationEmail(userEmail, verifyURL); err != nil {
		logger.Warn(ctx, "Failed to send verification email",
			"error", err.Error(),
			"user_id", user.ID,
			"email", userEmail,
		)
		logger.Info(ctx, fmt.Sprintf("Verification link for %s: %s", userEmail, verifyURL),
			"event", "user_registered",
			"user_id", user.ID,
			"email", userEmail,
		)
	} else {
		logger.Info(ctx, "Verification email sent",
			"event", "user_registered",
			"user_id", user.ID,
			"email", userEmail,
		)
	}

	return &user, nil
}

func (s *service) Login(ctx context.Context, email, password, ipAddress string) (LoginTokens, error) {
	var auth model.UserAuth
	if err := s.db.Where("identity_type = ? AND identifier = ?", "email", email).First(&auth).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return LoginTokens{}, errors.New("invalid credentials")
		}
		return LoginTokens{}, err
	}

	if !auth.CheckPassword(password) {
		return LoginTokens{}, errors.New("invalid credentials")
	}

	var user model.User
	if err := s.db.First(&user, auth.UserID).Error; err != nil {
		return LoginTokens{}, err
	}

	if user.Status == 2 {
		return LoginTokens{}, errors.New("please verify your email before logging in")
	}
	if user.Status == 1 {
		return LoginTokens{}, errors.New("your account has been suspended")
	}
	if err := s.hydrateMerchantRole(ctx, &user); err != nil {
		return LoginTokens{}, err
	}

	now := time.Now().UTC()
	auth.LastLoginAt = &now
	if err := s.db.Save(&auth).Error; err != nil {
		logger.Warn(ctx, "Failed to update user login info",
			"user_id", user.ID,
			"error", err.Error(),
		)
	}

	tokens, err := s.issueTokens(ctx, &user, &auth)
	if err != nil {
		return LoginTokens{}, err
	}

	logger.Info(ctx, "User logged in successfully",
		"event", "user_login_success",
		"user_id", user.ID,
	)

	_ = ipAddress
	return tokens, nil
}

func (s *service) RefreshAccessToken(ctx context.Context, refreshToken string) (LoginTokens, error) {
	if refreshToken == "" {
		return LoginTokens{}, errors.New("invalid refresh token")
	}

	tokenHash := token.HashToken(refreshToken)
	now := time.Now().UTC()

	var stored model.RefreshToken
	if err := s.db.Where("token_hash = ? AND revoked_at IS NULL", tokenHash).First(&stored).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return LoginTokens{}, errors.New("invalid refresh token")
		}
		return LoginTokens{}, err
	}
	if now.After(stored.ExpiresAt) {
		return LoginTokens{}, errors.New("invalid refresh token")
	}

	var auth model.UserAuth
	if err := s.db.Where("user_id = ? AND identity_type = ?", stored.UserID, "email").First(&auth).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return LoginTokens{}, errors.New("invalid refresh token")
		}
		return LoginTokens{}, err
	}

	var user model.User
	if err := s.db.First(&user, stored.UserID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return LoginTokens{}, errors.New("invalid refresh token")
		}
		return LoginTokens{}, err
	}
	if err := s.hydrateMerchantRole(ctx, &user); err != nil {
		return LoginTokens{}, err
	}

	var tokens LoginTokens
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.RefreshToken{}).
			Where("id = ? AND revoked_at IS NULL", stored.ID).
			Updates(map[string]interface{}{
				"revoked_at":   now,
				"last_used_at": now,
				"updated_at":   now,
			}).Error; err != nil {
			return err
		}

		issued, err := s.issueTokensInTx(tx, &user, &auth)
		if err != nil {
			return err
		}
		tokens = issued
		return nil
	}); err != nil {
		return LoginTokens{}, err
	}

	return tokens, nil
}

func (s *service) LoginOrRegisterOAuthUser(ctx context.Context, email, name, provider, avatar string) (string, error) {
	var auth model.UserAuth
	var user model.User

	err := s.db.Where("identity_type = ? AND identifier = ?", provider, email).First(&auth).Error
	if err == nil {
		if err := s.db.First(&user, auth.UserID).Error; err != nil {
			return "", err
		}
		if err := s.hydrateMerchantRole(ctx, &user); err != nil {
			return "", err
		}

		now := time.Now().UTC()
		auth.LastLoginAt = &now
		if err := s.db.Save(&auth).Error; err != nil {
			logger.Warn(ctx, "Failed to update OAuth user login info",
				"user_id", user.ID,
				"error", err.Error(),
			)
		}

		token, err := s.tokenService.GenerateToken(&user, &auth)
		if err != nil {
			return "", err
		}

		logger.Info(ctx, "OAuth user logged in successfully",
			"event", "oauth_login_success",
			"user_id", user.ID,
			"provider", provider,
		)

		return token, nil
	}

	if err != gorm.ErrRecordNotFound {
		return "", err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		user = model.User{Role: "user", Status: 0}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		now := time.Now().UTC()
		auth = model.UserAuth{
			UserID:       user.ID,
			IdentityType: provider,
			Identifier:   email,
			LastLoginAt:  &now,
		}
		if err := tx.Create(&auth).Error; err != nil {
			return err
		}

		profile := model.UserProfile{UserID: user.ID, Nickname: name, AvatarURL: avatar}
		if err := tx.Create(&profile).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", err
	}
	if err := s.hydrateMerchantRole(ctx, &user); err != nil {
		return "", err
	}

	token, err := s.tokenService.GenerateToken(&user, &auth)
	if err != nil {
		return "", err
	}

	logger.Info(ctx, "OAuth user registered and logged in successfully",
		"event", "oauth_register_success",
		"user_id", user.ID,
		"provider", provider,
	)

	return token, nil
}

// hydrateMerchantRole keeps the JWT principal aligned with merchant
// ownership. Older accounts can legitimately have users.role="user" even
// after a Merchant row has been created for them; relying only on the stored
// role makes merchant login and any role-gated merchant endpoint reject a
// valid owner. The source-of-truth relationship is read at authentication
// time without rewriting the user's persisted role.
func (s *service) hydrateMerchantRole(ctx context.Context, user *model.User) error {
	if user == nil || user.Role != "user" {
		return nil
	}

	var merchant model.Merchant
	err := s.db.WithContext(ctx).
		Select("id").
		Where("user_id = ?", user.ID).
		First(&merchant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	user.Role = "merchant"
	return nil
}

func (s *service) VerifyEmail(ctx context.Context, token string) error {
	var verification model.EmailVerification
	if err := s.db.Where("token = ?", token).First(&verification).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.New("invalid or expired verification token")
		}
		return err
	}

	if verification.IsExpired() {
		return errors.New("verification token has expired")
	}

	if err := s.db.Model(&model.User{}).Where("id = ?", verification.UserID).Update("status", 0).Error; err != nil {
		return fmt.Errorf("failed to activate user: %w", err)
	}

	if err := s.db.Delete(&verification).Error; err != nil {
		logger.Warn(ctx, "Failed to delete verification record",
			"error", err.Error(),
			"user_id", verification.UserID,
		)
	}

	logger.Info(ctx, "User email verified successfully",
		"event", "email_verified",
		"user_id", verification.UserID,
		"email", verification.Email,
	)

	return nil
}

func (s *service) issueTokens(ctx context.Context, user *model.User, auth *model.UserAuth) (LoginTokens, error) {
	var tokens LoginTokens
	err := s.db.Transaction(func(tx *gorm.DB) error {
		issued, err := s.issueTokensInTx(tx, user, auth)
		if err != nil {
			return err
		}
		tokens = issued
		return nil
	})
	if err != nil {
		return LoginTokens{}, err
	}
	return tokens, nil
}

func (s *service) issueTokensInTx(tx *gorm.DB, user *model.User, auth *model.UserAuth) (LoginTokens, error) {
	accessToken, err := s.tokenService.GenerateToken(user, auth)
	if err != nil {
		return LoginTokens{}, err
	}

	refreshToken, refreshHash, err := s.tokenService.GenerateRefreshToken()
	if err != nil {
		return LoginTokens{}, err
	}

	record := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: refreshHash,
		ExpiresAt: time.Now().UTC().Add(s.tokenService.RefreshTokenTTL()),
	}
	if err := tx.Create(&record).Error; err != nil {
		return LoginTokens{}, err
	}

	return LoginTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
