package handlersession

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domainauth "service-sender/internal/domain/auth"
	domainsession "service-sender/internal/domain/session"
	interfaceauth "service-sender/internal/interfaces/auth"
	interfacesession "service-sender/internal/interfaces/session"

	"github.com/gin-gonic/gin"
)

type sessionServiceStub struct {
	interfacesession.ServiceSessionInterface
	sessions     map[string]*domainsession.Session
	current      *domainsession.Session
	userSessions []*domainsession.SessionInfo
	callOrder    *[]string
	destroyed    string
}

func (s *sessionServiceStub) GetSessionBySessionID(_ context.Context, id string) (*domainsession.Session, error) {
	return s.sessions[id], nil
}

func (s *sessionServiceStub) GetSessionByToken(_ context.Context, _ string) (*domainsession.Session, error) {
	return s.current, nil
}

func (s *sessionServiceStub) GetUserSessions(_ context.Context, _, _ string) ([]*domainsession.SessionInfo, error) {
	return s.userSessions, nil
}

func (s *sessionServiceStub) DestroySession(_ context.Context, id string) error {
	*s.callOrder = append(*s.callOrder, "destroy:"+id)
	s.destroyed = id
	return nil
}

type blacklistRepoStub struct {
	interfaceauth.RepoAuthInterface
	tokens    []string
	callOrder *[]string
	err       error
}

func (r *blacklistRepoStub) Store(item domainauth.Blacklist) error {
	*r.callOrder = append(*r.callOrder, "blacklist:"+item.Token)
	if r.err != nil {
		return r.err
	}
	r.tokens = append(r.tokens, item.Token)
	return nil
}

func TestRevokeSessionBlacklistsTokenBeforeDeletingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	order := []string{}
	service := &sessionServiceStub{
		sessions: map[string]*domainsession.Session{
			"session-1": {SessionID: "session-1", UserID: "user-1", Token: "revoked-access-token"},
		},
		callOrder: &order,
	}
	blacklist := &blacklistRepoStub{callOrder: &order}
	handler := NewSessionHandler(service, blacklist)
	engine := gin.New()
	engine.DELETE("/sessions/:session_id", func(ctx *gin.Context) {
		ctx.Set("userId", "user-1")
		handler.RevokeSession(ctx)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/sessions/session-1", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if len(blacklist.tokens) != 1 || blacklist.tokens[0] != "revoked-access-token" {
		t.Fatalf("blacklisted tokens = %v", blacklist.tokens)
	}
	if service.destroyed != "session-1" {
		t.Fatalf("destroyed session = %q, want session-1", service.destroyed)
	}
	if len(order) != 2 || order[0] != "blacklist:revoked-access-token" || order[1] != "destroy:session-1" {
		t.Fatalf("call order = %v", order)
	}
}

func TestRevokeAllOtherSessionsBlacklistsOnlyOtherTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	order := []string{}
	service := &sessionServiceStub{
		sessions: map[string]*domainsession.Session{
			"other-1": {SessionID: "other-1", UserID: "user-1", Token: "other-access-token-1"},
			"other-2": {SessionID: "other-2", UserID: "user-1", Token: "other-access-token-2"},
		},
		current:      &domainsession.Session{SessionID: "current", UserID: "user-1", Token: "current-access-token"},
		userSessions: []*domainsession.SessionInfo{{SessionID: "current", IsCurrentSession: true}, {SessionID: "other-1"}, {SessionID: "other-2"}},
		callOrder:    &order,
	}
	blacklist := &blacklistRepoStub{callOrder: &order}
	handler := NewSessionHandler(service, blacklist)
	engine := gin.New()
	engine.POST("/sessions/revoke-others", func(ctx *gin.Context) {
		ctx.Set("userId", "user-1")
		ctx.Set("token", "current-access-token")
		handler.RevokeAllOtherSessions(ctx)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sessions/revoke-others", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	gotTokens := map[string]bool{}
	for _, token := range blacklist.tokens {
		gotTokens[token] = true
	}
	if len(gotTokens) != 2 || !gotTokens["other-access-token-1"] || !gotTokens["other-access-token-2"] || gotTokens["current-access-token"] {
		t.Fatalf("blacklisted tokens = %v", blacklist.tokens)
	}
	if len(order) != 4 || order[0] != "blacklist:other-access-token-1" || order[1] != "blacklist:other-access-token-2" || order[2] != "destroy:other-1" || order[3] != "destroy:other-2" {
		t.Fatalf("blacklisting and deletion order = %v", order)
	}
}

func TestRevokeSessionDoesNotDeleteWhenBlacklistFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	order := []string{}
	service := &sessionServiceStub{
		sessions: map[string]*domainsession.Session{
			"session-1": {SessionID: "session-1", UserID: "user-1", Token: "revoked-access-token"},
		},
		callOrder: &order,
	}
	handler := NewSessionHandler(service, &blacklistRepoStub{callOrder: &order, err: errors.New("database unavailable")})
	engine := gin.New()
	engine.DELETE("/sessions/:session_id", func(ctx *gin.Context) {
		ctx.Set("userId", "user-1")
		handler.RevokeSession(ctx)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/sessions/session-1", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if service.destroyed != "" {
		t.Fatalf("session deleted despite blacklist failure: %q", service.destroyed)
	}
}
