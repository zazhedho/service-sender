package handleruser

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	domainuser "service-sender/internal/domain/user"
	"service-sender/internal/dto"
	interfaceuser "service-sender/internal/interfaces/user"
	servicereset "service-sender/internal/services/reset"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type forgotService struct {
	interfaceuser.ServiceUserInterface
	called     bool
	missing    bool
	resetEmail string
}

func (s *forgotService) GetUserByEmail(email string) (domainuser.Users, error) {
	if s.missing {
		return domainuser.Users{}, gorm.ErrRecordNotFound
	}
	return domainuser.Users{Id: "user-1", Email: email}, nil
}

func (s *forgotService) ResetPasswordByEmail(email, password string) error {
	s.resetEmail = email
	return nil
}

type emailResetService struct {
	email    string
	token    string
	consumed bool
	err      error
}

func (s *emailResetService) RequestReset(_ context.Context, email, _ string) error {
	s.email = email
	return s.err
}
func (s *emailResetService) VerifyReset(_ context.Context, token string) (string, error) {
	if s.consumed || token != s.token {
		return "", errors.New("invalid reset token")
	}
	s.consumed = true
	return "user@example.com", nil
}

func TestProductionEmailResetFlow(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	gin.SetMode(gin.TestMode)
	svc := &forgotService{}
	email := &emailResetService{token: "private-email-token"}
	h := NewUserHandler(svc, nil)
	h.ResetService = email
	r := gin.New()
	r.POST("/forgot-password", h.ForgotPassword)
	r.POST("/reset-password", h.ResetPassword)
	request := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	w := request("/forgot-password", `{"email":"User@example.com"}`)
	if w.Code != http.StatusOK || email.email != "user@example.com" || svc.called || strings.Contains(w.Body.String(), "private-") {
		t.Fatalf("unsafe email reset response: %d %s", w.Code, w.Body.String())
	}
	for _, resetErr := range []error{errors.New("email delivery unavailable"), &servicereset.ThrottleError{Reason: "cooldown"}} {
		email.err = resetErr
		w = request("/forgot-password", `{"email":"user@example.com"}`)
		if w.Code != http.StatusOK || w.Header().Get("Retry-After") != "" || !strings.Contains(w.Body.String(), "Password reset instructions") {
			t.Fatal("delivery error or throttling revealed account existence")
		}
	}
	email.err = nil
	knownResponse := w.Body.String()
	svc.missing = true
	email.email = ""
	w = request("/forgot-password", `{"email":"missing@example.com"}`)
	// Ignore per-request log_id when comparing the public response.
	if w.Code != http.StatusOK || email.email != "" || !strings.Contains(knownResponse, "Password reset instructions") || !strings.Contains(w.Body.String(), "Password reset instructions") {
		t.Fatal("unknown email disclosed or sent")
	}
	w = request("/reset-password", `{"token":"private-email-token","new_password":"weakpass"}`)
	if w.Code != http.StatusBadRequest || email.consumed {
		t.Fatal("invalid password consumed reset token")
	}
	w = request("/reset-password", `{"token":"private-email-token","new_password":"NewPassword1!"}`)
	if w.Code != http.StatusOK || svc.resetEmail != "user@example.com" {
		t.Fatalf("reset failed: %d %s", w.Code, w.Body.String())
	}
	w = request("/reset-password", `{"token":"private-email-token","new_password":"OtherPassword1!"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatal("used email token accepted")
	}
}

func TestDevelopmentFallbackUsesResetToken(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("JWT_KEY", "test-only-key-for-password-reset-regression")
	// Real token generation belongs to the user service; verify the explicit fallback remains available.
	svc := &forgotService{}
	r := gin.New()
	r.POST("/forgot-password", NewUserHandler(svc, nil).ForgotPassword)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !svc.called {
		t.Fatal("development fallback unavailable")
	}
}

func (s *forgotService) ForgotPassword(dto.ForgotPasswordRequest) (string, error) {
	s.called = true
	return "private-reset-token", nil
}

func TestForgotPasswordProductionDoesNotExposeToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, env := range []string{"production", "staging", ""} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			svc := &forgotService{}
			r := gin.New()
			r.POST("/forgot-password", NewUserHandler(svc, nil).ForgotPassword)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(`{"email":"user@example.com"}`))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("expected 503 without email service, got %d", w.Code)
			}
			if svc.called || strings.Contains(w.Body.String(), "private-reset-token") {
				t.Error("production fallback generated or exposed reset token")
			}
		})
	}
}
