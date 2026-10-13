package handleruser

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	domainuser "service-sender/internal/domain/user"
	"service-sender/internal/dto"
	interfaceuser "service-sender/internal/interfaces/user"
	"service-sender/utils"

	"github.com/gin-gonic/gin"
)

const credentialLogHelper = "SERVICE_SENDER_CREDENTIAL_LOG_HELPER"

type credentialLoggingServiceStub struct {
	interfaceuser.ServiceUserInterface
}

func (*credentialLoggingServiceStub) RegisterUser(dto.UserRegister) (domainuser.Users, error) {
	return domainuser.Users{}, nil
}

func (*credentialLoggingServiceStub) AdminCreateUser(dto.AdminCreateUser, string) (domainuser.Users, error) {
	return domainuser.Users{}, nil
}

func (*credentialLoggingServiceStub) LoginUser(dto.Login, string) (string, error) {
	return "jwt-test-sentinel", nil
}

func TestCredentialRequestsAndTokensAreNotLogged(t *testing.T) {
	if os.Getenv(credentialLogHelper) == "1" {
		runCredentialLogProbe(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCredentialRequestsAndTokensAreNotLogged$")
	cmd.Env = credentialTestEnv(os.Environ(), map[string]string{
		credentialLogHelper: "1", "LOG_FORMAT": "text", "LOG_LEVEL": "5",
	})
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("credential log helper failed: %v\n%s", err, output)
	}
	for _, secret := range []string{"register-password-sentinel", "admin-password-sentinel", "login-password-sentinel", "jwt-test-sentinel"} {
		if strings.Contains(string(output), secret) {
			t.Errorf("logs exposed %q: %s", secret, output)
		}
	}
}

func credentialTestEnv(env []string, replacements map[string]string) []string {
	result := make([]string, 0, len(env)+len(replacements))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[key]; !replaced {
			result = append(result, entry)
		}
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

func runCredentialLogProbe(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := NewUserHandler(&credentialLoggingServiceStub{}, nil)
	if got := serveCredentialRequest(handler.Register, `{"name":"Test User","email":"register@example.com","phone":"08123456789","password":"register-password-sentinel"}`, nil); got != http.StatusCreated {
		t.Fatalf("register status = %d", got)
	}
	if got := serveCredentialRequest(handler.AdminCreateUser, `{"name":"Test User","email":"admin@example.com","password":"admin-password-sentinel","role":"user"}`, map[string]any{"role": "admin"}); got != http.StatusCreated {
		t.Fatalf("admin create status = %d", got)
	}
	if got := serveCredentialRequest(handler.Login, `{"email":"login@example.com","password":"login-password-sentinel"}`, nil); got != http.StatusOK {
		t.Fatalf("login status = %d", got)
	}
}

func serveCredentialRequest(handler gin.HandlerFunc, body string, auth map[string]any) int {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if auth != nil {
		ctx.Set(utils.CtxKeyAuthData, auth)
	}
	handler(ctx)
	return recorder.Code
}
