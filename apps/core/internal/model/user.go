package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User 核心用户表 (只存不可变/系统级信息)
type User struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Role      string    `gorm:"type:varchar(20);not null;default:'user'" json:"role"` // 'user', 'admin'
	Status    int16     `gorm:"not null;default:0" json:"status"`                     // 0: active, 1: banned, 2: pending
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Auths   []UserAuth   `gorm:"foreignKey:UserID" json:"auths,omitempty"`
	Profile *UserProfile `gorm:"foreignKey:UserID" json:"profile,omitempty"`
}

func (u *User) TableName() string {
	return "users"
}

// UserAuth 用户认证表 (支持多重登录方式：Email/Google/Apple)
type UserAuth struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       int64      `gorm:"not null;index" json:"user_id"`
	IdentityType string     `gorm:"type:varchar(20);not null" json:"identity_type"` // 'email', 'google', 'apple'
	Identifier   string     `gorm:"type:varchar(255);not null" json:"identifier"`   // email地址 或 open_id/sub
	Credential   string     `gorm:"type:varchar(255)" json:"-"`                     // 密码hash 或 access_token
	LastLoginAt  *time.Time `json:"last_login_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (ua *UserAuth) TableName() string {
	return "user_auths"
}

func (ua *UserAuth) SetPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	ua.Credential = string(hashedPassword)
	return nil
}

func (ua *UserAuth) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(ua.Credential), []byte(password))
	return err == nil
}

// UserProfile 用户画像表 (业务展示信息)
type UserProfile struct {
	UserID    int64  `gorm:"primaryKey" json:"user_id"`
	Nickname  string `gorm:"type:varchar(50);not null" json:"nickname"`
	AvatarURL string `gorm:"type:varchar(255)" json:"avatar_url"`
	Intro     string `gorm:"type:varchar(255)" json:"intro"` // 一句话简介
	Location  string `gorm:"type:varchar(100)" json:"location"`
	// Stats (denormalized)
	FollowerCount  int `gorm:"default:0" json:"follower_count"`
	FollowingCount int `gorm:"default:0" json:"following_count"`
	PostCount      int `gorm:"default:0" json:"post_count"`
	ReviewCount    int `gorm:"default:0" json:"review_count"`
	LikeCount      int `gorm:"default:0" json:"like_count"`
	CouponCount    int `gorm:"default:0" json:"coupon_count"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (up *UserProfile) TableName() string {
	return "user_profiles"
}

// EmailVerification stores email verification tokens
type EmailVerification struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64     `gorm:"not null;index" json:"user_id"`
	Email     string    `gorm:"type:varchar(255);not null" json:"email"`
	Token     string    `gorm:"type:varchar(255);not null;uniqueIndex" json:"token"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (ev *EmailVerification) TableName() string {
	return "email_verifications"
}

func (ev *EmailVerification) IsExpired() bool {
	return time.Now().UTC().After(ev.ExpiresAt)
}

// PasswordResetToken stores a hashed, single-use password reset token.
// The raw token is only sent to the account's email address and is never
// persisted in the database.
type PasswordResetToken struct {
	ID        int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64      `gorm:"not null;index" json:"user_id"`
	TokenHash string     `gorm:"type:char(64);not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `gorm:"not null;index" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `gorm:"autoCreateTime;index" json:"created_at"`

	User *User `gorm:"foreignKey:UserID" json:"-"`
}

func (prt *PasswordResetToken) TableName() string {
	return "password_reset_tokens"
}

func (prt *PasswordResetToken) IsExpired() bool {
	return time.Now().UTC().After(prt.ExpiresAt)
}
