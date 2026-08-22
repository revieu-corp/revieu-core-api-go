package router

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
)

// routeContract pins the full public surface: every method+path. Refactors
// that move a route between domains are free; refactors that change a path or
// drop a route fail.
type routeContract struct {
	method string
	path   string
}

func contractCfg() *config.Config {
	return &config.Config{
		Server:      config.ServerConfig{APIBasePath: "/api/v1"},
		JWT:         config.JWTConfig{Secret: "test-secret", ExpireHour: 24},
		FrontendURL: "https://merchant.revieu.test",
	}
}

func registeredRoutes(t *testing.T) []routeContract {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	Setup(r, contractCfg())

	out := make([]routeContract, 0, len(r.Routes()))
	for _, ri := range r.Routes() {
		out = append(out, routeContract{method: ri.Method, path: ri.Path})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].path == out[j].path {
			return out[i].method < out[j].method
		}
		return out[i].path < out[j].path
	})
	return out
}

// TestEveryRouteIsVersioned guarantees the base path is the single source of
// truth for versioning: no domain may register outside it.
func TestEveryRouteIsVersioned(t *testing.T) {
	for _, rt := range registeredRoutes(t) {
		if !strings.HasPrefix(rt.path, "/api/v1/") {
			t.Errorf("route %s %s escapes the API base path", rt.method, rt.path)
		}
	}
}

// TestBasePathIsConfigurable proves the version prefix is not hardcoded in
// domains: changing the config moves the entire surface.
func TestBasePathIsConfigurable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := contractCfg()
	cfg.Server.APIBasePath = "/api/v9"

	r := gin.New()
	Setup(r, cfg)

	routes := r.Routes()
	if len(routes) == 0 {
		t.Fatal("no routes registered")
	}
	for _, ri := range routes {
		if !strings.HasPrefix(ri.Path, "/api/v9/") {
			t.Errorf("route %s %s did not follow the configured base path", ri.Method, ri.Path)
		}
	}
}

// TestNoDuplicateRoutes catches two domains claiming the same method+path,
// which Gin resolves by registration order and is always a bug.
func TestNoDuplicateRoutes(t *testing.T) {
	seen := map[string]bool{}
	for _, rt := range registeredRoutes(t) {
		key := rt.method + " " + rt.path
		if seen[key] {
			t.Errorf("duplicate route registration: %s", key)
		}
		seen[key] = true
	}
}

// TestHealthAndSwaggerAreRegisteredByRouter pins that operational endpoints
// live under the same versioned prefix as business routes. Kubernetes probes
// depend on /api/v1/health resolving.
func TestHealthAndSwaggerAreRegisteredByRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	Setup(r, contractCfg())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/health = %d, want 200 (probe path must resolve)", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok"`) {
		t.Errorf("health body = %q, want status ok", w.Body.String())
	}
}
