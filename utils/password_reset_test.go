package utils

import (
	"github.com/golang-jwt/jwt/v5"
	domainuser "service-sender/internal/domain/user"
	"testing"
	"time"
)

func TestJWTRequiresConfiguredKey(t *testing.T) {
	t.Setenv("JWT_KEY", "")
	if _, err := GenerateJwt(&domainuser.Users{Id: "user-1"}, "login"); err == nil {
		t.Fatal("JWT signed without a key")
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "user-1", "token_type": "access", "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := JwtClaim(token); err == nil {
		t.Fatal("JWT verified without a key")
	}
}

func TestResetJWTRequiresPositiveTTL(t *testing.T) {
	t.Setenv("JWT_KEY", "test-only-key-for-password-reset-regression")
	for _, ttl := range []string{"0", "-1"} {
		t.Setenv("RESET_TTL_SECONDS", ttl)
		if _, err := GeneratePasswordResetJwt(&domainuser.Users{Id: "user-1"}); err == nil {
			t.Fatalf("invalid TTL accepted: %s", ttl)
		}
	}
}
