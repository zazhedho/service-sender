package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAPIKeyMiddlewareFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/send", APIKeyMiddleware("EMAIL_API_KEY"), func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	for _, tc := range []struct {
		name, configured, supplied string
		want                       int
	}{
		{name: "missing config", want: http.StatusUnauthorized},
		{name: "missing key", configured: "secret", want: http.StatusUnauthorized},
		{name: "wrong key", configured: "secret", supplied: "wrong", want: http.StatusUnauthorized},
		{name: "valid key", configured: "secret", supplied: "secret", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EMAIL_API_KEY", tc.configured)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/send", nil)
			if tc.supplied != "" {
				req.Header.Set("X-API-Key", tc.supplied)
			}
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, w.Code)
			}
		})
	}
}
