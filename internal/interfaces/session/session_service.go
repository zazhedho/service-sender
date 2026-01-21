package interfacesession

import (
	"context"

	domainsession "starter-kit/internal/domain/session"
	domainuser "starter-kit/internal/domain/user"

	"github.com/gin-gonic/gin"
)

type ServiceSessionInterface interface {
	CreateSession(ctx context.Context, user *domainuser.Users, token string, ginCtx *gin.Context) (*domainsession.Session, error)
	ValidateSession(ctx context.Context, token string) (*domainsession.Session, error)
	GetUserSessions(ctx context.Context, userID string, currentSessionID string) ([]*domainsession.SessionInfo, error)
	DestroySession(ctx context.Context, sessionID string) error
	DestroySessionByToken(ctx context.Context, token string) error
	DestroyAllUserSessions(ctx context.Context, userID string) error
	DestroyOtherSessions(ctx context.Context, userID string, currentSessionID string) error
	GetSessionByToken(ctx context.Context, token string) (*domainsession.Session, error)
	GetSessionBySessionID(ctx context.Context, sessionID string) (*domainsession.Session, error)
}
