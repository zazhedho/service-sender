package middlewares

import (
	"crypto/subtle"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"service-sender/pkg/messages"
	"service-sender/pkg/response"
	"service-sender/utils"
)

func APIKeyMiddleware(envKey string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		expected := os.Getenv(envKey)
		provided := ctx.GetHeader("X-API-Key")
		if expected == "" || len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			logId := utils.GenerateLogId(ctx)
			res := response.Response(http.StatusUnauthorized, messages.MsgDenied, logId, nil)
			res.Error = "Invalid API key"
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, res)
			return
		}
		ctx.Next()
	}
}
