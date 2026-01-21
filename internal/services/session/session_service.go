package servicesession

import (
	"context"
	"fmt"
	domainsession "starter-kit/internal/domain/session"
	domainuser "starter-kit/internal/domain/user"
	interfacesession "starter-kit/internal/interfaces/session"
	"starter-kit/utils"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ServiceSession struct {
	SessionRepo interfacesession.RepoSessionInterface
}

func NewSessionService(sessionRepo interfacesession.RepoSessionInterface) *ServiceSession {
	return &ServiceSession{
		SessionRepo: sessionRepo,
	}
}

func (s *ServiceSession) CreateSession(ctx context.Context, user *domainuser.Users, token string, ginCtx *gin.Context) (*domainsession.Session, error) {
	sessionID := uuid.New().String()

	jwtExpHours := utils.GetEnv("JWT_EXP", 24).(int)
	expiresAt := time.Now().Add(time.Hour * time.Duration(jwtExpHours))

	deviceInfo := extractDeviceInfo(ginCtx)
	ip := ginCtx.ClientIP()
	userAgent := ginCtx.GetHeader("User-Agent")

	session := &domainsession.Session{
		SessionID:    sessionID,
		UserID:       user.Id,
		Email:        user.Email,
		Role:         user.Role,
		Token:        token,
		DeviceInfo:   deviceInfo,
		IP:           ip,
		UserAgent:    userAgent,
		LoginAt:      time.Now(),
		LastActivity: time.Now(),
		ExpiresAt:    expiresAt,
	}

	if err := s.SessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

func (s *ServiceSession) ValidateSession(ctx context.Context, token string) (*domainsession.Session, error) {
	session, err := s.SessionRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("session not found or expired")
	}

	if time.Now().After(session.ExpiresAt) {
		s.SessionRepo.Delete(ctx, session.SessionID)
		return nil, fmt.Errorf("session expired")
	}

	if err := s.SessionRepo.UpdateActivity(ctx, session.SessionID); err != nil {
		fmt.Printf("Failed to update session activity: %v\n", err)
	}

	return session, nil
}

func (s *ServiceSession) GetUserSessions(ctx context.Context, userID string, currentSessionID string) ([]*domainsession.SessionInfo, error) {
	sessions, err := s.SessionRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	sessionInfos := make([]*domainsession.SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		info := &domainsession.SessionInfo{
			SessionID:        session.SessionID,
			DeviceInfo:       session.DeviceInfo,
			IP:               session.IP,
			LoginAt:          session.LoginAt,
			LastActivity:     session.LastActivity,
			IsCurrentSession: session.SessionID == currentSessionID,
		}
		sessionInfos = append(sessionInfos, info)
	}

	return sessionInfos, nil
}

func (s *ServiceSession) DestroySession(ctx context.Context, sessionID string) error {
	return s.SessionRepo.Delete(ctx, sessionID)
}

func (s *ServiceSession) DestroySessionByToken(ctx context.Context, token string) error {
	session, err := s.SessionRepo.GetByToken(ctx, token)
	if err != nil {
		return err
	}
	return s.SessionRepo.Delete(ctx, session.SessionID)
}

func (s *ServiceSession) GetSessionByToken(ctx context.Context, token string) (*domainsession.Session, error) {
	return s.SessionRepo.GetByToken(ctx, token)
}

func (s *ServiceSession) GetSessionBySessionID(ctx context.Context, sessionID string) (*domainsession.Session, error) {
	return s.SessionRepo.GetBySessionID(ctx, sessionID)
}

func (s *ServiceSession) DestroyAllUserSessions(ctx context.Context, userID string) error {
	return s.SessionRepo.DeleteByUserID(ctx, userID)
}

func (s *ServiceSession) DestroyOtherSessions(ctx context.Context, userID string, currentSessionID string) error {
	sessions, err := s.SessionRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session.SessionID != currentSessionID {
			if err := s.SessionRepo.Delete(ctx, session.SessionID); err != nil {
				return err
			}
		}
	}

	return nil
}

func extractDeviceInfo(ctx *gin.Context) string {
	userAgent := ctx.GetHeader("User-Agent")

	if contains(userAgent, "Mobile") || contains(userAgent, "Android") || contains(userAgent, "iPhone") {
		if contains(userAgent, "Android") {
			return "Android Mobile"
		} else if contains(userAgent, "iPhone") {
			return "iOS Mobile"
		}
		return "Mobile Device"
	} else if contains(userAgent, "iPad") || contains(userAgent, "Tablet") {
		return "Tablet"
	} else if contains(userAgent, "Windows") {
		return "Windows PC"
	} else if contains(userAgent, "Macintosh") || contains(userAgent, "Mac OS") {
		return "Mac"
	} else if contains(userAgent, "Linux") {
		return "Linux"
	}

	return "Unknown Device"
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

var _ interfacesession.ServiceSessionInterface = (*ServiceSession)(nil)
