package handleruser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"starter-kit/infrastructure/database"
	"starter-kit/internal/dto"
	interfaceuser "starter-kit/internal/interfaces/user"
	sessionRepo "starter-kit/internal/repositories/session"
	sessionSvc "starter-kit/internal/services/session"
	"starter-kit/pkg/filter"
	"starter-kit/pkg/logger"
	"starter-kit/pkg/messages"
	"starter-kit/pkg/response"
	"starter-kit/pkg/security"
	"starter-kit/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type HandlerUser struct {
	Service      interfaceuser.ServiceUserInterface
	LoginLimiter security.LoginLimiter
}

func NewUserHandler(s interfaceuser.ServiceUserInterface, limiter security.LoginLimiter) *HandlerUser {
	return &HandlerUser{
		Service:      s,
		LoginLimiter: limiter,
	}
}

func (h *HandlerUser) Register(ctx *gin.Context) {
	var req dto.UserRegister
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][Register]"

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))

		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Request: %+v;", logPrefix, utils.JsonEncode(req)))

	data, err := h.Service.RegisterUser(req)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.RegisterUser; Error: %+v", logPrefix, err))
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Error: email or phone already exists", logPrefix))
			res := response.Response(http.StatusBadRequest, messages.MsgExists, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadRequest, Message: "email or phone already exists"}
			ctx.JSON(http.StatusBadRequest, res)
			return
		}

		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusCreated, "User registered successfully", logId, data)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(data)))
	ctx.JSON(http.StatusCreated, res)
}

func (h *HandlerUser) AdminCreateUser(ctx *gin.Context) {
	var req dto.AdminCreateUser
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][AdminCreateUser]"

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))
		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Request: %+v;", logPrefix, utils.JsonEncode(req)))

	// Get creator's role from auth data
	authData := utils.GetAuthData(ctx)
	if authData == nil {
		res := response.Response(http.StatusUnauthorized, "Unauthorized", logId, nil)
		ctx.JSON(http.StatusUnauthorized, res)
		return
	}
	creatorRole := authData["role"].(string)

	data, err := h.Service.AdminCreateUser(req, creatorRole)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.AdminCreateUser; Error: %+v", logPrefix, err))
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "already exists") {
			res := response.Response(http.StatusBadRequest, messages.MsgExists, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadRequest, Message: err.Error()}
			ctx.JSON(http.StatusBadRequest, res)
			return
		}

		res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	res := response.Response(http.StatusCreated, "User created successfully", logId, data)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(data)))
	ctx.JSON(http.StatusCreated, res)
}

func (h *HandlerUser) Login(ctx *gin.Context) {
	var req dto.Login
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserController][Login]"

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))

		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Request: %+v;", logPrefix, utils.JsonEncode(req)))

	loginIdentifier := fmt.Sprintf("%s:%s", ctx.ClientIP(), strings.ToLower(req.Email))
	if h.LoginLimiter != nil {
		blocked, ttl, limiterErr := h.LoginLimiter.IsBlocked(ctx.Request.Context(), loginIdentifier)
		if limiterErr != nil {
			logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; LoginLimiter.IsBlocked error: %v", logPrefix, limiterErr))
		} else if blocked {
			logger.WriteLogWithContext(ctx, logger.LogLevelWarn, fmt.Sprintf("%s; Too many attempts", logPrefix))
			h.respondTooManyLoginAttempts(ctx, logId, ttl)
			return
		}
	}

	token, err := h.Service.LoginUser(req, logId.String())
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.LoginUser; ERROR: %s;", logPrefix, err))
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == messages.ErrHashPassword {
			if h.LoginLimiter != nil {
				blocked, ttl, limiterErr := h.LoginLimiter.RegisterFailure(ctx.Request.Context(), loginIdentifier)
				if limiterErr != nil {
					logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; LoginLimiter.RegisterFailure error: %v", logPrefix, limiterErr))
				}
				if blocked {
					logger.WriteLogWithContext(ctx, logger.LogLevelWarn, fmt.Sprintf("%s; Account temporarily locked after repeated failures", logPrefix))
					h.respondTooManyLoginAttempts(ctx, logId, ttl)
					return
				}
			}

			res := response.Response(http.StatusBadRequest, messages.InvalidCred, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadRequest, Message: messages.MsgCredential}
			ctx.JSON(http.StatusBadRequest, res)
			return
		}

		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	if h.LoginLimiter != nil {
		if err := h.LoginLimiter.Reset(ctx.Request.Context(), loginIdentifier); err != nil {
			logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; LoginLimiter.Reset error: %v", logPrefix, err))
		}
	}

	// Create session if Redis is available
	if redisClient := database.GetRedisClient(); redisClient != nil {
		user, errUser := h.Service.GetUserByEmail(req.Email)
		if errUser == nil {
			sRepo := sessionRepo.NewSessionRepository(redisClient)
			sSvc := sessionSvc.NewSessionService(sRepo)

			session, errSession := sSvc.CreateSession(context.Background(), &user, token, ctx)
			if errSession != nil {
				logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Failed to create session: %v", logPrefix, errSession))
			} else {
				logger.WriteLogWithContext(ctx, logger.LogLevelInfo, fmt.Sprintf("%s; Session created: %s", logPrefix, session.SessionID))
			}
		}
	}

	res := response.Response(http.StatusOK, "success", logId, map[string]interface{}{"token": token})
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(token)))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) respondTooManyLoginAttempts(ctx *gin.Context, logId uuid.UUID, ttl time.Duration) {
	if ttl > 0 {
		ctx.Header("Retry-After", strconv.Itoa(int(ttl.Seconds())))
	}

	message := "Too many login attempts. Please try again later."
	if ttl > 0 {
		message = fmt.Sprintf("Too many login attempts. Try again in %d seconds.", int(ttl.Seconds()))
	}

	res := response.Response(http.StatusTooManyRequests, messages.MsgFail, logId, nil)
	res.Error = response.Errors{Code: http.StatusTooManyRequests, Message: message}
	ctx.AbortWithStatusJSON(http.StatusTooManyRequests, res)
}

func (h *HandlerUser) Logout(ctx *gin.Context) {
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserController][Logout]"

	token, ok := ctx.Get("token")
	if !ok {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; token not found in context", logPrefix))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = "token not found"
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	// Destroy session if Redis is available
	if redisClient := database.GetRedisClient(); redisClient != nil {
		sRepo := sessionRepo.NewSessionRepository(redisClient)
		sSvc := sessionSvc.NewSessionService(sRepo)

		errSession := sSvc.DestroySessionByToken(context.Background(), token.(string))
		if errSession != nil {
			logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Failed to destroy session: %v", logPrefix, errSession))
		} else {
			logger.WriteLogWithContext(ctx, logger.LogLevelInfo, fmt.Sprintf("%s; Session destroyed successfully", logPrefix))
		}
	}

	if err := h.Service.LogoutUser(token.(string)); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.LogoutUser; Error: %+v", logPrefix, err))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusOK, "User logged out successfully", logId, nil)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Success: User logged out successfully", logPrefix))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) GetUserById(ctx *gin.Context) {
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][GetUserByID]"

	id, err := utils.ValidateUUID(ctx, logId)
	if err != nil {
		return
	}

	data, err := h.Service.GetUserById(id)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.GetUserByID; ERROR: %s;", logPrefix, err))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res := response.Response(http.StatusNotFound, messages.MsgNotFound, logId, nil)
			res.Error = response.Errors{Code: http.StatusNotFound, Message: "user not found"}
			ctx.JSON(http.StatusNotFound, res)
			return
		}

		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusOK, "success", logId, data)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(data)))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) GetUserByAuth(ctx *gin.Context) {
	authData := utils.GetAuthData(ctx)
	userId := utils.InterfaceString(authData["user_id"])
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][GetUserByAuth]"

	data, err := h.Service.GetUserByAuth(userId)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.GetUserByAuth; ERROR: %s;", logPrefix, err))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res := response.Response(http.StatusNotFound, messages.MsgNotFound, logId, nil)
			res.Error = response.Errors{Code: http.StatusNotFound, Message: "user not found"}
			ctx.JSON(http.StatusNotFound, res)
			return
		}

		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusOK, "success", logId, data)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(data)))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) GetAllUsers(ctx *gin.Context) {
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][GetAllUsers]"

	authData := utils.GetAuthData(ctx)
	currentUserRole := utils.InterfaceString(authData["role"])

	params, _ := filter.GetBaseParams(ctx, "updated_at", "desc", 10)
	params.Filters = filter.WhitelistFilter(params.Filters, []string{"role"})

	users, totalData, err := h.Service.GetAllUsers(params, currentUserRole)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; GetAllUsers; ERROR: %+v;", logPrefix, err))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.PaginationResponse(http.StatusOK, int(totalData), params.Page, params.Limit, logId, users)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(users)))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) Update(ctx *gin.Context) {
	var req dto.UserUpdate
	authData := utils.GetAuthData(ctx)
	userId := utils.InterfaceString(authData["user_id"])
	role := utils.InterfaceString(authData["role"])
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][Update]"

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))

		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	data, err := h.Service.Update(userId, role, req)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.Update; ERROR: %s;", logPrefix, err))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res := response.Response(http.StatusNotFound, messages.MsgNotFound, logId, nil)
			res.Error = response.Errors{Code: http.StatusNotFound, Message: "user not found"}
			ctx.JSON(http.StatusNotFound, res)
			return
		}

		res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	res := response.Response(http.StatusOK, "User updated successfully", logId, data)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(data)))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) UpdateUserById(ctx *gin.Context) {
	var req dto.UserUpdate
	authData := utils.GetAuthData(ctx)
	role := utils.InterfaceString(authData["role"])
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][UpdateUserById]"

	id, err := utils.ValidateUUID(ctx, logId)
	if err != nil {
		return
	}

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))

		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Request: %+v;", logPrefix, utils.JsonEncode(req)))

	data, err := h.Service.Update(id, role, req)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.Update; ERROR: %s;", logPrefix, err))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res := response.Response(http.StatusNotFound, messages.MsgNotFound, logId, nil)
			res.Error = response.Errors{Code: http.StatusNotFound, Message: "user not found"}
			ctx.JSON(http.StatusNotFound, res)
			return
		}

		res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	res := response.Response(http.StatusOK, "User updated successfully", logId, data)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(data)))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) ChangePassword(ctx *gin.Context) {
	var req dto.ChangePassword
	authData := utils.GetAuthData(ctx)
	userId := utils.InterfaceString(authData["user_id"])
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][ChangePassword]"

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))

		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	data, err := h.Service.ChangePassword(userId, req)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.ChangePassword; ERROR: %s;", logPrefix, err))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res := response.Response(http.StatusNotFound, messages.MsgNotFound, logId, nil)
			res.Error = response.Errors{Code: http.StatusNotFound, Message: "user not found"}
			ctx.JSON(http.StatusNotFound, res)
			return
		}

		if err.Error() == messages.ErrHashPassword {
			res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
			res.Error = response.Errors{Code: http.StatusBadRequest, Message: "current password is incorrect"}
			ctx.JSON(http.StatusBadRequest, res)
			return
		}

		res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	res := response.Response(http.StatusOK, "User password changed successfully", logId, data)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Response: %+v;", logPrefix, utils.JsonEncode(data)))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) ForgotPassword(ctx *gin.Context) {
	var req dto.ForgotPasswordRequest
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][ForgotPassword]"

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))
		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	token, err := h.Service.ForgotPassword(req)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.ForgotPassword; ERROR: %s;", logPrefix, err))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	logger.WriteLogWithContext(ctx, logger.LogLevelInfo, fmt.Sprintf("MOCK EMAIL SENT: Reset Token for %s: %s", req.Email, token))

	res := response.Response(http.StatusOK, "Password reset instructions sent to your email", logId, token)
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) ResetPassword(ctx *gin.Context) {
	var req dto.ResetPasswordRequest
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][ResetPassword]"

	if err := ctx.BindJSON(&req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))
		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	if err := h.Service.ResetPassword(req); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.ResetPassword; ERROR: %s;", logPrefix, err))
		res := response.Response(http.StatusBadRequest, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	res := response.Response(http.StatusOK, "Password reset successfully", logId, nil)
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) Delete(ctx *gin.Context) {
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][Delete]"
	authData := utils.GetAuthData(ctx)
	userId := utils.InterfaceString(authData["user_id"])

	if err := h.Service.Delete(userId); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.Delete; ERROR: %s;", logPrefix, err))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res := response.Response(http.StatusNotFound, messages.MsgNotFound, logId, nil)
			res.Error = response.Errors{Code: http.StatusNotFound, Message: "user not found"}
			ctx.JSON(http.StatusNotFound, res)
			return
		}

		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusOK, "User deleted successfully", logId, nil)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Success: User deleted successfully", logPrefix))
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerUser) DeleteUserById(ctx *gin.Context) {
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[UserHandler][DeleteUserById]"

	id, err := utils.ValidateUUID(ctx, logId)
	if err != nil {
		return
	}

	if err := h.Service.Delete(id); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.Delete; ERROR: %s;", logPrefix, err))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res := response.Response(http.StatusNotFound, messages.MsgNotFound, logId, nil)
			res.Error = response.Errors{Code: http.StatusNotFound, Message: "user not found"}
			ctx.JSON(http.StatusNotFound, res)
			return
		}

		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusOK, "User deleted successfully", logId, nil)
	logger.WriteLogWithContext(ctx, logger.LogLevelDebug, fmt.Sprintf("%s; Success: User deleted successfully", logPrefix))
	ctx.JSON(http.StatusOK, res)
}
