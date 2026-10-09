package utils

import (
	"errors"
	"fmt"
	"os"
	domainuser "service-sender/internal/domain/user"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AppClaims struct {
	UserId    string `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	*jwt.RegisteredClaims
}

const (
	TokenTypeAccess        = "access"
	TokenTypePasswordReset = "password_reset"
)

func GenerateJwt(user *domainuser.Users, logId string) (string, error) {
	return generateJwt(user, logId, TokenTypeAccess, time.Hour*time.Duration(GetEnv("JWT_EXP", 24).(int)))
}

func GeneratePasswordResetJwt(user *domainuser.Users) (string, error) {
	ttl := GetEnv("RESET_TTL_SECONDS", 900).(int)
	if ttl <= 0 {
		return "", errors.New("reset token TTL must be positive")
	}
	return generateJwt(user, CreateUUID(), TokenTypePasswordReset, time.Duration(ttl)*time.Second)
}

func generateJwt(user *domainuser.Users, logId, tokenType string, ttl time.Duration) (string, error) {
	if os.Getenv("JWT_KEY") == "" {
		return "", errors.New("jwt key is not configured")
	}
	claims := AppClaims{
		UserId:    user.Id,
		Username:  user.Name,
		Role:      user.Role,
		TokenType: tokenType,
		RegisteredClaims: &jwt.RegisteredClaims{
			ID:        logId,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)

	signedToken, err := token.SignedString([]byte(os.Getenv("JWT_KEY")))
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

func GetAuthToken(ctx *gin.Context) string {
	bearerToken := ctx.Request.Header.Get("Authorization")
	return strings.ReplaceAll(bearerToken, "Bearer ", "")
}

func JwtClaims(ctx *gin.Context) (string, map[string]interface{}, error) {
	tokenString := GetAuthToken(ctx)
	data, err := JwtClaim(tokenString)
	return tokenString, data, err
}

func JwtClaim(tokenString string) (map[string]interface{}, error) {
	if tokenString == "" {
		return nil, errors.New("empty token")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		hmacSecretString := GetEnv("JWT_KEY", "").(string)
		if hmacSecretString == "" {
			return nil, errors.New("jwt key is not configured")
		}
		hmacSecret := []byte(hmacSecretString)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return "", fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return hmacSecret, nil
	})

	if err != nil || !token.Valid {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, err
}
