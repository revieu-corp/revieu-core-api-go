package authorization

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		userID   int64
		role     string
		expected int
	}{
		{name: "admin", userID: 1, role: "admin", expected: http.StatusNoContent},
		{name: "regular-user", userID: 1, role: "user", expected: http.StatusForbidden},
		{name: "missing-principal", role: "admin", expected: http.StatusUnauthorized},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if testCase.userID > 0 {
					c.Set(UserIDKey, testCase.userID)
				}
				c.Set(UserRoleKey, testCase.role)
				c.Next()
			}, RequireRole("admin"))
			router.GET("/admin", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/admin", nil)
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.expected {
				t.Fatalf("expected status %d, got %d: %s", testCase.expected, recorder.Code, recorder.Body.String())
			}
		})
	}
}
