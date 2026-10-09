package middlewares

import (
	"net/http"
	"net/http/httptest"
	domainauth "service-sender/internal/domain/auth"
	interfaceauth "service-sender/internal/interfaces/auth"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

type emptyBlacklist struct {
	interfaceauth.RepoAuthInterface
}

func (*emptyBlacklist) GetByToken(string) (domainauth.Blacklist, error) {
	return domainauth.Blacklist{}, gorm.ErrRecordNotFound
}

func TestAuthRejectsNonAccessTokens(t *testing.T) {
	t.Setenv("JWT_KEY", "test-only-key-for-password-reset-regression")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/private", NewMiddleware(&emptyBlacklist{}, nil).AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })
	for _, kind := range []string{"password_reset", "refresh", "", "access"} {
		t.Run(kind, func(t *testing.T) {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "user-1", "token_type": kind, "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte("test-only-key-for-password-reset-regression"))
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/private", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			r.ServeHTTP(w, req)
			want := http.StatusUnauthorized
			if kind == "access" {
				want = http.StatusOK
			}
			if w.Code != want {
				t.Errorf("token type %q: expected %d, got %d", kind, want, w.Code)
			}
		})
	}
}
