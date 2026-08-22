package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/authorization"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/logger"
)

const (
	oauthStateCookieName = "revieu_oauth_state"
	oauthStateMaxAge     = 10 * 60
)

type Handler struct {
	svc           Service
	oauthCfg      config.OAuthConfig
	frontendURL   string
	apiBasePath   string
	jwtExpireHour int
}

func NewHandler(jwtCfg config.JWTConfig, oauthCfg config.OAuthConfig, smtpCfg config.SMTPConfig, frontendURL string, apiBasePath string) *Handler {
	return &Handler{
		svc:           NewService(nil, jwtCfg, smtpCfg),
		oauthCfg:      oauthCfg,
		frontendURL:   frontendURL,
		apiBasePath:   apiBasePath,
		jwtExpireHour: jwtCfg.ExpireHour,
	}
}

func (h *Handler) frontendRedirectURL() string {
	configured := strings.TrimRight(strings.TrimSpace(h.frontendURL), "/")
	if parsed, err := url.Parse(configured); err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return configured
	}
	return "http://localhost:3000"
}

func (h *Handler) authCookiePath() string {
	base := strings.TrimRight(strings.TrimSpace(h.apiBasePath), "/")
	if base == "" {
		return "/auth"
	}
	return base + "/auth"
}

func (h *Handler) apiCookiePath() string {
	base := strings.TrimRight(strings.TrimSpace(h.apiBasePath), "/")
	if base == "" {
		return "/"
	}
	return base
}

func (h *Handler) secureCookie(c *gin.Context) bool {
	return h.requestScheme(c) == "https"
}

func newOAuthState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func (h *Handler) setOAuthStateCookie(c *gin.Context, state string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     h.authCookiePath(),
		MaxAge:   oauthStateMaxAge,
		HttpOnly: true,
		Secure:   h.secureCookie(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearOAuthStateCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     h.authCookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookie(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) consumeOAuthState(c *gin.Context) (string, error) {
	state := c.Query("state")
	cookie, err := c.Request.Cookie(oauthStateCookieName)
	h.clearOAuthStateCookie(c)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookie.Value)) != 1 {
		return "", fmt.Errorf("invalid oauth state")
	}
	return h.frontendRedirectURL(), nil
}

func (h *Handler) setAccessTokenCookie(c *gin.Context, accessToken string) {
	maxAge := h.jwtExpireHour * 60 * 60
	if maxAge <= 0 {
		maxAge = 60 * 60
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     authorization.AccessTokenCookieName,
		Value:    accessToken,
		Path:     h.apiCookiePath(),
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.secureCookie(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearAccessTokenCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     authorization.AccessTokenCookieName,
		Value:    "",
		Path:     h.apiCookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookie(c),
		SameSite: http.SameSiteLaxMode,
	})
}

// requestScheme returns the external protocol for building OAuth
// redirect_uri. Prefer the configured frontend URL's scheme (always https
// in production) because X-Forwarded-Proto is unreliable behind the
// Cloudflare tunnel + traefik path, which sets it to http for the internal
// hop. Fall back to X-Forwarded-Proto / TLS detection for local dev.
func (h *Handler) requestScheme(c *gin.Context) string {
	if u, err := url.Parse(h.frontendURL); err == nil && u.Scheme == "https" {
		return "https"
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); proto == "https" {
		return "https"
	}
	if c.Request.TLS != nil {
		return "https"
	}
	return "http"
}

// googleRedirectURI builds the callback URL for Google OAuth. It must match
// the Authorized redirect URI registered in Google Cloud Console exactly.
func (h *Handler) googleRedirectURI(c *gin.Context) string {
	scheme := h.requestScheme(c)
	return fmt.Sprintf("%s://%s%s/auth/callback/google", scheme, c.Request.Host, h.apiBasePath)
}

// Register godoc
// @Summary Register a new user
// @Description Register a new user with username, email and password
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Register Request"
// @Success 201 {object} RegisterResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	baseURL := scheme + "://" + c.Request.Host + h.apiBasePath

	user, err := h.svc.Register(c.Request.Context(), req.Username, req.Email, req.Password, baseURL)
	if err != nil {
		logger.Error(c.Request.Context(), "Registration failed",
			"error", err.Error(),
			"event", "user_register_failed",
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, RegisterResponse{
		Message: "User created successfully. Please check your email for verification link (printed in server logs for now).",
		UserID:  user.ID,
	})
}

// Login godoc
// @Summary Login user
// @Description Login with email and password to get JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login Request"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ipAddress := c.ClientIP()
	tokens, err := h.svc.Login(c.Request.Context(), req.Email, req.Password, ipAddress)
	if err != nil {
		logger.Warn(c.Request.Context(), "Login failed",
			"error", err.Error(),
			"email", req.Email,
			"event", "user_login_failed",
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token:        tokens.AccessToken,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		Type:         "Bearer",
	})
}

// GoogleLogin godoc
// @Summary Redirect to Google OAuth
// @Description Redirects user to Google OAuth authorization page
// @Tags auth
// @Success 302 "Redirect to Google OAuth"
// @Router /auth/login/google [get]
func (h *Handler) GoogleLogin(c *gin.Context) {
	clientID := h.oauthCfg.Google.ClientID
	if clientID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Google OAuth not configured"})
		return
	}

	redirectURI := h.googleRedirectURI(c)
	state, err := newOAuthState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize OAuth state"})
		return
	}
	h.setOAuthStateCookie(c, state)

	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"access_type":   {"offline"},
		"state":         {state},
	}
	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()

	c.Redirect(http.StatusFound, authURL)
}

// GoogleCallback godoc
// @Summary Handle Google OAuth callback
// @Description Handles Google OAuth callback, creates/logs in user, redirects to frontend with token
// @Tags auth
// @Param code query string true "Authorization code from Google"
// @Success 302 "Redirect to frontend with token"
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/callback/google [get]
func (h *Handler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
		return
	}

	frontendURL, err := h.consumeOAuthState(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid OAuth state"})
		return
	}

	redirectURI := h.googleRedirectURI(c)

	tokenResp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {h.oauthCfg.Google.ClientID},
		"client_secret": {h.oauthCfg.Google.ClientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		logger.Error(c.Request.Context(), "Failed to exchange code for token",
			"error", err.Error(),
			"event", "google_oauth_token_exchange_failed",
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange authorization code"})
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil {
		logger.Error(c.Request.Context(), "Failed to decode token response",
			"error", err.Error(),
			"event", "google_oauth_token_decode_failed",
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode token response"})
		return
	}

	if tokenData.Error != "" {
		logger.Error(c.Request.Context(), "Google OAuth error",
			"error", tokenData.Error,
			"event", "google_oauth_error",
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": tokenData.Error})
		return
	}

	userInfoResp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + tokenData.AccessToken)
	if err != nil {
		logger.Error(c.Request.Context(), "Failed to get user info from Google",
			"error", err.Error(),
			"event", "google_oauth_userinfo_failed",
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
		return
	}
	defer userInfoResp.Body.Close()

	body, err := io.ReadAll(userInfoResp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read user info"})
		return
	}

	var userInfo struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &userInfo); err != nil {
		logger.Error(c.Request.Context(), "Failed to decode user info",
			"error", err.Error(),
			"event", "google_oauth_userinfo_decode_failed",
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode user info"})
		return
	}

	token, err := h.svc.LoginOrRegisterOAuthUser(c.Request.Context(), userInfo.Email, userInfo.Name, "google", userInfo.Picture)
	if err != nil {
		logger.Error(c.Request.Context(), "Failed to login/register OAuth user",
			"error", err.Error(),
			"event", "google_oauth_login_failed",
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process login"})
		return
	}

	h.setAccessTokenCookie(c, token)
	redirectURL := fmt.Sprintf("%s/auth/callback", frontendURL)
	c.Redirect(http.StatusFound, redirectURL)
}

// Logout godoc
// @Summary Clear the current browser session
// @Description Clears the HttpOnly OAuth access-token cookie
// @Tags auth
// @Success 204
// @Router /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	h.clearAccessTokenCookie(c)
	c.JSON(http.StatusNoContent, nil)
}

// ForgotPassword godoc
// @Summary Send password reset email
// @Description Sends a password reset email if the account exists
// @Tags auth
// @Accept json
// @Produce json
// @Param request body ForgotPasswordRequest true "Forgot Password Request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /auth/forgot-password [post]
func (h *Handler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email"})
		return
	}
	// Placeholder: do not reveal if the user exists.
	c.JSON(http.StatusOK, gin.H{})
}

// VerifyEmail godoc
// @Summary Verify user email
// @Description Verify user email using the token sent to their email
// @Tags auth
// @Param token query string true "Verification token"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /auth/verify [get]
func (h *Handler) VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing verification token"})
		return
	}

	if err := h.svc.VerifyEmail(c.Request.Context(), token); err != nil {
		logger.Warn(c.Request.Context(), "Email verification failed",
			"error", err.Error(),
			"event", "email_verification_failed",
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	frontendURL := h.frontendURL
	if frontendURL == "" {
		if referer := c.GetHeader("Referer"); referer != "" {
			if parsedURL, err := url.Parse(referer); err == nil {
				frontendURL = fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
			}
		} else if origin := c.GetHeader("Origin"); origin != "" {
			frontendURL = origin
		} else {
			frontendURL = "http://localhost:3000"
		}
	}
	redirectURL := fmt.Sprintf("%s/auth/verified", frontendURL)
	c.Redirect(http.StatusFound, redirectURL)
}

// Me godoc
// @Summary Get current user info
// @Description Get the current authenticated user's information (protected route)
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} UserInfoResponse
// @Failure 401 {object} map[string]string
// @Router /auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	userID, _ := c.Get("user_id")
	email, _ := c.Get("user_email")
	role, _ := c.Get("user_role")

	c.JSON(http.StatusOK, UserInfoResponse{
		UserID:  userID,
		Email:   email,
		Role:    role,
		Message: "Token is valid!",
	})
}

// Refresh godoc
// @Summary Refresh access token
// @Description Rotates refresh token and returns a new access token pair
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RefreshRequest true "Refresh Request"
// @Success 200 {object} RefreshResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.svc.RefreshAccessToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	c.JSON(http.StatusOK, RefreshResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		Type:         "Bearer",
	})
}
