package handlersession

import (
	"context"
	"fmt"
	"net/http"
	domainauth "service-sender/internal/domain/auth"
	interfaceauth "service-sender/internal/interfaces/auth"
	interfacesession "service-sender/internal/interfaces/session"
	"service-sender/pkg/logger"
	"service-sender/pkg/messages"
	"service-sender/pkg/response"
	"service-sender/utils"
	"time"

	"github.com/gin-gonic/gin"
)

type HandlerSession struct {
	Service       interfacesession.ServiceSessionInterface
	BlacklistRepo interfaceauth.RepoAuthInterface
}

const userNotAuthenticatedMessage = "user not authenticated"

func NewSessionHandler(s interfacesession.ServiceSessionInterface, blacklistRepo interfaceauth.RepoAuthInterface) *HandlerSession {
	return &HandlerSession{Service: s, BlacklistRepo: blacklistRepo}
}

func (h *HandlerSession) GetActiveSessions(ctx *gin.Context) {
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[SessionHandler][GetActiveSessions]"

	userID, ok := ctx.Get("userId")
	if !ok {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; userId not found in context", logPrefix))
		res := response.Response(http.StatusUnauthorized, messages.MsgFail, logId, nil)
		res.Error = userNotAuthenticatedMessage
		ctx.JSON(http.StatusUnauthorized, res)
		return
	}

	token, _ := ctx.Get("token")
	currentSession, err := h.Service.GetSessionByToken(context.Background(), token.(string))
	currentSessionID := ""
	if err == nil && currentSession != nil {
		currentSessionID = currentSession.SessionID
	}

	sessions, err := h.Service.GetUserSessions(context.Background(), userID.(string), currentSessionID)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.GetUserSessions; Error: %+v", logPrefix, err))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusOK, "success", logId, map[string]interface{}{
		"sessions": sessions,
		"total":    len(sessions),
	})
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerSession) RevokeSession(ctx *gin.Context) {
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[SessionHandler][RevokeSession]"

	sessionID := ctx.Param("session_id")
	if sessionID == "" {
		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logId, nil)
		res.Error = "session_id is required"
		ctx.JSON(http.StatusBadRequest, res)
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; userId not found in context", logPrefix))
		res := response.Response(http.StatusUnauthorized, messages.MsgFail, logId, nil)
		res.Error = userNotAuthenticatedMessage
		ctx.JSON(http.StatusUnauthorized, res)
		return
	}

	session, err := h.Service.GetSessionBySessionID(context.Background(), sessionID)
	if err != nil || session == nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.GetSessionBySessionID; Error: %+v", logPrefix, err))
		res := response.Response(http.StatusNotFound, messages.MsgFail, logId, nil)
		res.Error = "session not found"
		ctx.JSON(http.StatusNotFound, res)
		return
	}

	if session.UserID != userID.(string) {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; unauthorized session revocation attempt", logPrefix))
		res := response.Response(http.StatusForbidden, messages.MsgDenied, logId, nil)
		res.Error = "unauthorized"
		ctx.JSON(http.StatusForbidden, res)
		return
	}
	if session.Token == "" {
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = "session token is unavailable"
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}
	if err := h.blacklistSession(session.Token); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; failed to blacklist session token", logPrefix))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = "failed to revoke session"
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	if err := h.Service.DestroySession(context.Background(), sessionID); err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.DestroySession; Error: %+v", logPrefix, err))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = err.Error()
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	res := response.Response(http.StatusOK, "Session revoked successfully", logId, nil)
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerSession) RevokeAllOtherSessions(ctx *gin.Context) {
	logId := utils.GenerateLogId(ctx)
	logPrefix := "[SessionHandler][RevokeAllOtherSessions]"

	userID, ok := ctx.Get("userId")
	if !ok {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; userId not found in context", logPrefix))
		res := response.Response(http.StatusUnauthorized, messages.MsgFail, logId, nil)
		res.Error = userNotAuthenticatedMessage
		ctx.JSON(http.StatusUnauthorized, res)
		return
	}

	token, _ := ctx.Get("token")
	currentSession, err := h.Service.GetSessionByToken(context.Background(), token.(string))
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.GetSessionByToken; Error: %+v", logPrefix, err))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = "failed to get current session"
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}
	if currentSession == nil || currentSession.UserID != userID.(string) {
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = "failed to get current session"
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}

	sessions, err := h.Service.GetUserSessions(context.Background(), userID.(string), currentSession.SessionID)
	if err != nil {
		logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.GetUserSessions; Error: %+v", logPrefix, err))
		res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
		res.Error = "failed to get user sessions"
		ctx.JSON(http.StatusInternalServerError, res)
		return
	}
	otherSessionIDs := make([]string, 0, len(sessions))
	for _, info := range sessions {
		if info == nil || info.SessionID == currentSession.SessionID {
			continue
		}
		session, err := h.Service.GetSessionBySessionID(context.Background(), info.SessionID)
		if err != nil || session == nil || session.UserID != userID.(string) || session.Token == "" {
			logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; failed to load other session for revocation", logPrefix))
			res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
			res.Error = "failed to revoke other sessions"
			ctx.JSON(http.StatusInternalServerError, res)
			return
		}
		if err := h.blacklistSession(session.Token); err != nil {
			logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; failed to blacklist other session token", logPrefix))
			res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
			res.Error = "failed to revoke other sessions"
			ctx.JSON(http.StatusInternalServerError, res)
			return
		}
		otherSessionIDs = append(otherSessionIDs, session.SessionID)
	}

	for _, sessionID := range otherSessionIDs {
		if err := h.Service.DestroySession(context.Background(), sessionID); err != nil {
			logger.WriteLogWithContext(ctx, logger.LogLevelError, fmt.Sprintf("%s; Service.DestroySession; Error: %+v", logPrefix, err))
			res := response.Response(http.StatusInternalServerError, messages.MsgFail, logId, nil)
			res.Error = err.Error()
			ctx.JSON(http.StatusInternalServerError, res)
			return
		}
	}

	res := response.Response(http.StatusOK, "All other sessions revoked successfully", logId, nil)
	ctx.JSON(http.StatusOK, res)
}

func (h *HandlerSession) blacklistSession(token string) error {
	if h.BlacklistRepo == nil {
		return fmt.Errorf("blacklist repository unavailable")
	}
	return h.BlacklistRepo.Store(domainauth.Blacklist{ID: utils.CreateUUID(), Token: token, CreatedAt: time.Now()})
}
