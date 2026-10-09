package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmailRoutesReturnsUnavailableWithoutSMTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("EMAIL_API_KEY", "test-api-key")
	clearSMTPConfig(t)
	routes := NewRoutes()
	routes.EmailRoutes()

	response := postJSON(routes.App, "/api/email/send", `{"type":"notification","to":["user@example.com"],"subject":"Hello","text_body":"Test"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
}

func TestPasswordResetEmailRouteReturnsUnavailableWithoutSMTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("EMAIL_API_KEY", "test-api-key")
	clearSMTPConfig(t)
	routes := NewRoutes()
	routes.PasswordResetRoutes()

	response := postJSON(routes.App, "/api/auth/reset-password/email", `{"email":"user@example.com","token":"test-token"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
}

func clearSMTPConfig(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SMTP_HOST", "SMTP_PASS", "SMTP_FROM"} {
		t.Setenv(key, "")
	}
}

func postJSON(engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-Key", "test-api-key")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}
