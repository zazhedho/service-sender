package handleremail

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"

	"service-sender/internal/dto"
	interfaceemail "service-sender/internal/interfaces/email"
	serviceemail "service-sender/internal/services/email"
	"service-sender/pkg/logger"
	"service-sender/pkg/messages"
	"service-sender/pkg/response"
	"service-sender/utils"
)

type HandlerEmail struct {
	Service interfaceemail.ServiceEmailInterface
}

func NewEmailHandler(s interfaceemail.ServiceEmailInterface) *HandlerEmail {
	return &HandlerEmail{Service: s}
}

func (h *HandlerEmail) SendEmail(ctx *gin.Context) {
	var req dto.SendEmailRequest
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[EmailHandler][SendEmail]"

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, logPrefix+"; BindJSON ERROR: "+err.Error())
		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	appName := strings.TrimSpace(ctx.GetHeader("X-App-Name"))
	if appName == "" {
		appName = strings.TrimSpace(utils.GetEnv("EMAIL_APP_NAME", utils.GetEnv("OTP_APP_NAME", "Account Verification").(string)).(string))
	}

	total, subject, err := h.Service.Send(ctx.Request.Context(), req, appName)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.Send error: %v", logPrefix, err))
		switch {
		case errors.Is(err, serviceemail.ErrEmailNotConfigured):
			res := response.Response(http.StatusServiceUnavailable, messages.MsgFail, logId, nil)
			res.Error = response.Errors{Code: http.StatusServiceUnavailable, Message: "Email sender is not available"}
			ctx.JSON(http.StatusServiceUnavailable, res)
			return
		case errors.Is(err, serviceemail.ErrSubjectRequired):
			res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadRequest, Message: "subject is required when template_key is empty"}
			ctx.JSON(http.StatusBadRequest, res)
			return
		case errors.Is(err, serviceemail.ErrEmailBodyRequired):
			res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadRequest, Message: "text_body or html_body is required"}
			ctx.JSON(http.StatusBadRequest, res)
			return
		case errors.Is(err, serviceemail.ErrTemplateNotFound):
			res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadRequest, Message: "template_key is not registered"}
			ctx.JSON(http.StatusBadRequest, res)
			return
		default:
			res := response.Response(http.StatusBadGateway, messages.MsgFail, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadGateway, Message: "Failed to send email"}
			ctx.JSON(http.StatusBadGateway, res)
			return
		}
	}

	res := response.Response(http.StatusOK, messages.MsgSuccess, logId, gin.H{
		"type":       req.Type,
		"subject":    subject,
		"total_sent": total,
	})
	ctx.JSON(http.StatusOK, res)
}
