package handlerreset

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"service-sender/internal/dto"
	interfacereset "service-sender/internal/interfaces/reset"
	servicereset "service-sender/internal/services/reset"
	"service-sender/pkg/config"
	"service-sender/pkg/logger"
	"service-sender/pkg/mailer"
	"service-sender/pkg/messages"
	"service-sender/pkg/response"
	"service-sender/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type HandlerReset struct {
	Service interfacereset.ServicePasswordResetInterface
	Sender  mailer.PasswordResetSender
	Config  config.PasswordResetConfig
}

const (
	passwordResetServiceUnavailable = "Password reset service is not available"
	bindJSONErrorPrefix             = "; BindJSON ERROR: "
)

func NewResetHandler(s interfacereset.ServicePasswordResetInterface, sender mailer.PasswordResetSender, cfg config.PasswordResetConfig) *HandlerReset {
	return &HandlerReset{Service: s, Sender: sender, Config: cfg}
}

func writePasswordResetUnavailable(ctx *gin.Context, logId uuid.UUID, status int) {
	res := response.Response(status, messages.MsgFail, logId, nil)
	res.Error = response.Errors{Code: status, Message: passwordResetServiceUnavailable}
	ctx.JSON(status, res)
}

func bindResetJSON(ctx *gin.Context, req interface{}, logId uuid.UUID, logPrefix string) bool {
	if err := ctx.BindJSON(req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, logPrefix+bindJSONErrorPrefix+err.Error())
		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req).Elem(), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return false
	}
	return true
}

func (h *HandlerReset) RequestReset(ctx *gin.Context) {
	var req dto.PasswordResetRequest
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[ResetHandler][RequestReset]"

	if h.Service == nil {
		writePasswordResetUnavailable(ctx, logId, http.StatusServiceUnavailable)
		return
	}

	if !bindResetJSON(ctx, &req, logId, logPrefix) {
		return
	}

	appName := strings.TrimSpace(ctx.GetHeader("X-App-Name"))
	if appName == "" {
		appName = strings.TrimSpace(utils.GetEnv("RESET_APP_NAME", utils.GetEnv("OTP_APP_NAME", "Account Verification").(string)).(string))
	}

	err := h.Service.RequestReset(ctx.Request.Context(), req.Email, appName)
	if err != nil {
		if throttle := new(servicereset.ThrottleError); errors.As(err, &throttle) {
			retryAfter := int(throttle.RetryAfter.Seconds())
			if retryAfter > 0 {
				ctx.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			res := response.Response(http.StatusTooManyRequests, messages.MsgFail, logId, nil)
			res.Error = response.Errors{Code: http.StatusTooManyRequests, Message: "Please wait before requesting another reset email"}
			ctx.JSON(http.StatusTooManyRequests, res)
			return
		}

		if errors.Is(err, servicereset.ErrResetNotConfigured) || errors.Is(err, servicereset.ErrResetDeliveryFailed) {
			writePasswordResetUnavailable(ctx, logId, http.StatusInternalServerError)
			return
		}

		res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
		res.Error = response.Errors{Code: http.StatusBadRequest, Message: "Unable to process reset request"}
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	res := response.Response(http.StatusOK, messages.MsgSuccess, logId, map[string]string{"email": req.Email})
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerReset) VerifyReset(ctx *gin.Context) {
	var req dto.PasswordResetVerifyRequest
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[ResetHandler][VerifyReset]"

	if h.Service == nil {
		writePasswordResetUnavailable(ctx, logId, http.StatusServiceUnavailable)
		return
	}

	if !bindResetJSON(ctx, &req, logId, logPrefix) {
		return
	}

	email, err := h.Service.VerifyReset(ctx.Request.Context(), req.Token)
	if err != nil {
		if errors.Is(err, servicereset.ErrResetInvalid) {
			res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadRequest, Message: "Invalid or expired token"}
			ctx.JSON(http.StatusBadRequest, res)
			return
		}

		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.VerifyReset error: %v", logPrefix, err))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = response.Errors{Code: http.StatusInternalServerError, Message: "Unable to verify token"}
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusOK, messages.MsgSuccess, logId, map[string]string{"email": email})
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerReset) SendResetEmail(ctx *gin.Context) {
	var req dto.PasswordResetEmailRequest
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[ResetHandler][SendResetEmail]"

	if h.Sender == nil {
		writePasswordResetUnavailable(ctx, logId, http.StatusServiceUnavailable)
		return
	}

	if !bindResetJSON(ctx, &req, logId, logPrefix) {
		return
	}

	appName := strings.TrimSpace(ctx.GetHeader("X-App-Name"))
	if appName == "" {
		appName = strings.TrimSpace(utils.GetEnv("RESET_APP_NAME", utils.GetEnv("OTP_APP_NAME", "Account Verification").(string)).(string))
	}

	ttl := h.Config.TTL
	if req.ExpiresInMinutes > 0 {
		ttl = time.Duration(req.ExpiresInMinutes) * time.Minute
	}

	resetURL := strings.TrimSpace(req.ResetURL)
	if resetURL == "" {
		resetURL = buildResetURL(h.Config.URLTemplate, req.Token)
	}

	if err := h.Sender.SendPasswordReset(req.Email, req.Token, appName, resetURL, ttl); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; SendPasswordReset error: %v", logPrefix, err))
		res := response.Response(http.StatusBadGateway, messages.MsgFail, logId, nil)
		res.Error = response.Errors{Code: http.StatusBadGateway, Message: "Failed to send reset email"}
		ctx.JSON(http.StatusBadGateway, res)
		return
	}

	res := response.Response(http.StatusOK, messages.MsgSuccess, logId, map[string]string{"email": req.Email})
	ctx.JSON(http.StatusOK, res)
}

func buildResetURL(template, token string) string {
	if template == "" {
		return ""
	}
	if strings.Contains(template, "{token}") {
		return strings.ReplaceAll(template, "{token}", token)
	}
	if strings.Contains(template, "?") {
		return template + "&token=" + token
	}
	return template + "?token=" + token
}
