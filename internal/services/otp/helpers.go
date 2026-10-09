package serviceotp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func generateOTP() (string, error) {
	upperBound := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, upperBound)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func hashOTP(code, secret string) string {
	h := sha256.Sum256([]byte(code + secret))
	return hex.EncodeToString(h[:])
}

func verifyOTP(code, hashed, secret string) bool {
	candidate := hashOTP(code, secret)
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(hashed)) == 1
}
