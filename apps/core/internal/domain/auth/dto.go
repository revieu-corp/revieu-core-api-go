package auth

import "github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"

// RegisterRequest registers a new user.
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// LoginRequest logs in an existing user.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// ForgotPasswordRequest requests password reset email.
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ResetPasswordRequest consumes a password reset token.
type ResetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
}

// RegisterResponse is returned on successful registration.
type RegisterResponse struct {
	Message string `json:"message"`
	UserID  int64  `json:"user_id"`
}

// LoginResponse is returned on successful login.
type LoginResponse struct {
	Token        string `json:"token,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

// RefreshRequest requests a new token pair.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RefreshResponse returns a rotated token pair.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

// UserInfoResponse describes the authenticated user.
type UserInfoResponse struct {
	UserID  interface{} `json:"user_id"`
	Email   interface{} `json:"email"`
	Role    interface{} `json:"role"`
	Message string      `json:"message"`
}

// ToUserResponse maps model.User to the auth response.
func ToUserResponse(user *model.User) UserInfoResponse {
	return UserInfoResponse{
		UserID:  user.ID,
		Email:   "",
		Role:    user.Role,
		Message: "Token is valid!",
	}
}
