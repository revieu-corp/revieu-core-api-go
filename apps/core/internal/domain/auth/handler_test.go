package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
)

type stubAuthService struct {
	refreshFn func(context.Context, string) (LoginTokens, error)
}

func (s stubAuthService) Register(context.Context, string, string, string, string) (*model.User, error) {
	return nil, errors.New("not implemented")
}

func (s stubAuthService) Login(context.Context, string, string, string) (LoginTokens, error) {
	return LoginTokens{}, errors.New("not implemented")
}

func (s stubAuthService) RefreshAccessToken(ctx context.Context, refreshToken string) (LoginTokens, error) {
	return s.refreshFn(ctx, refreshToken)
}

func (s stubAuthService) LoginOrRegisterOAuthUser(context.Context, string, string, string, string) (string, error) {
	return "", errors.New("not implemented")
}

func (s stubAuthService) VerifyEmail(context.Context, string) error {
	return errors.New("not implemented")
}

func TestRefreshHandlerSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{
		svc: stubAuthService{
			refreshFn: func(_ context.Context, refreshToken string) (LoginTokens, error) {
				if refreshToken != "valid-refresh" {
					return LoginTokens{}, errors.New("invalid refresh token")
				}
				return LoginTokens{
					AccessToken:  "new-access",
					RefreshToken: "new-refresh",
				}, nil
			},
		},
	}

	r := gin.New()
	r.POST("/auth/refresh", h.Refresh)

	body, _ := json.Marshal(RefreshRequest{RefreshToken: "valid-refresh"})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp RefreshResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.AccessToken != "new-access" {
		t.Fatalf("expected access token new-access, got %q", resp.AccessToken)
	}
	if resp.RefreshToken != "new-refresh" {
		t.Fatalf("expected refresh token new-refresh, got %q", resp.RefreshToken)
	}
	if resp.Type != "Bearer" {
		t.Fatalf("expected type Bearer, got %q", resp.Type)
	}
}

func TestRefreshHandlerInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{
		svc: stubAuthService{
			refreshFn: func(_ context.Context, refreshToken string) (LoginTokens, error) {
				return LoginTokens{}, errors.New("invalid refresh token")
			},
		},
	}

	r := gin.New()
	r.POST("/auth/refresh", h.Refresh)

	body, _ := json.Marshal(RefreshRequest{RefreshToken: "bad-refresh"})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequestSchemePrefersFrontendHTTPS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{frontendURL: "https://revieu.liweijun.com"}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/login/google", nil)
	c.Request.Header.Set("X-Forwarded-Proto", "http")
	if s := h.requestScheme(c); s != "https" {
		t.Fatalf("requestScheme=%q, want https", s)
	}
}

func TestRequestSchemeFallsBackToProto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{frontendURL: ""}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	if s := h.requestScheme(c); s != "https" {
		t.Fatalf("requestScheme=%q, want https", s)
	}
}

func TestGoogleLoginUsesOpaqueSessionBoundState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{
		oauthCfg:    config.OAuthConfig{Google: config.GoogleOAuthConfig{ClientID: "client-id"}},
		frontendURL: "https://revieu.example",
		apiBasePath: "/api/v1",
	}

	r := gin.New()
	r.GET("/api/v1/auth/login/google", h.GoogleLogin)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login/google", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("failed to parse OAuth location: %v", err)
	}
	state := location.Query().Get("state")
	if len(state) != 64 || state == "https://revieu.example" {
		t.Fatalf("expected opaque 32-byte state, got %q", state)
	}

	var stateCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == oauthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected OAuth state cookie")
	}
	if stateCookie.Value != state || !stateCookie.HttpOnly || stateCookie.Path != "/api/v1/auth" {
		t.Fatalf("unexpected state cookie: %+v", stateCookie)
	}
	if stateCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", stateCookie.SameSite)
	}
}

func TestConsumeOAuthStateRequiresCookieAndClearsIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{frontendURL: "https://revieu.example", apiBasePath: "/api/v1"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback/google?state=opaque-state", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "opaque-state"})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	frontendURL, err := h.consumeOAuthState(c)
	if err != nil {
		t.Fatalf("consumeOAuthState returned error: %v", err)
	}
	if frontendURL != "https://revieu.example" {
		t.Fatalf("frontend URL = %q", frontendURL)
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), oauthStateCookieName+"=;") {
		t.Fatalf("expected state cookie deletion, got %q", rec.Header().Get("Set-Cookie"))
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback/google?state=attacker-state", nil)
	badReq.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "real-state"})
	badRec := httptest.NewRecorder()
	badContext, _ := gin.CreateTestContext(badRec)
	badContext.Request = badReq
	if _, err := h.consumeOAuthState(badContext); err == nil {
		t.Fatal("expected mismatched OAuth state to be rejected")
	}
}

func TestGoogleCallbackRejectsMissingOAuthStateBeforeTokenExchange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{
		oauthCfg:    config.OAuthConfig{Google: config.GoogleOAuthConfig{ClientID: "client-id"}},
		frontendURL: "https://revieu.example",
		apiBasePath: "/api/v1",
	}

	r := gin.New()
	r.GET("/api/v1/auth/callback/google", h.GoogleCallback)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback/google?code=authorization-code", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid OAuth state") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestLogoutClearsAccessTokenCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{apiBasePath: "/api/v1"}
	r := gin.New()
	r.POST("/api/v1/auth/logout", h.Logout)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "revieu_access_token=;") {
		t.Fatalf("expected access cookie deletion, got %q", rec.Header().Get("Set-Cookie"))
	}
}
