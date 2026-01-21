package utils

import (
	"errors"
	"fmt"
	"os"
	domainuser "starter-kit/internal/domain/user"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AppClaims struct {
	UserId   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	*jwt.RegisteredClaims
}

func GenerateJwt(user *domainuser.Users, logId string) (string, error) {
	claims := AppClaims{
		UserId:   user.Id,
		Username: user.Name,
		Role:     user.Role,
		RegisteredClaims: &jwt.RegisteredClaims{
			ID:        logId,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * time.Duration(GetEnv("JWT_EXP", 24).(int)))),
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
