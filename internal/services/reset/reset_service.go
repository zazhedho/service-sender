package servicereset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	interfacereset "service-sender/internal/interfaces/reset"
	"service-sender/pkg/config"
	"service-sender/pkg/mailer"

	"github.com/redis/go-redis/v9"
)

var (
	ErrResetInvalid        = errors.New("reset token invalid or expired")
	ErrResetNotConfigured  = errors.New("password reset service not configured")
	ErrResetDeliveryFailed = errors.New("password reset delivery failed")
)

type ThrottleError struct {
	Reason     string
	RetryAfter time.Duration
}

func (e *ThrottleError) Error() string {
	if e == nil {
		return "reset throttled"
	}
	return fmt.Sprintf("reset throttled: %s", e.Reason)
}

type ServiceReset struct {
	Repo   interfacereset.RepoPasswordResetInterface
	Sender mailer.PasswordResetSender
	Config config.PasswordResetConfig
}

func NewPasswordResetService(repo interfacereset.RepoPasswordResetInterface, sender mailer.PasswordResetSender, cfg config.PasswordResetConfig) *ServiceReset {
	return &ServiceReset{
		Repo:   repo,
		Sender: sender,
		Config: cfg,
	}
}

func (s *ServiceReset) RequestReset(ctx context.Context, email, appName string) error {
	if s == nil || s.Repo == nil || s.Sender == nil {
		return ErrResetNotConfigured
	}

	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		return ErrResetInvalid
	}

	cooldownTTL, err := s.Repo.GetCooldownTTL(ctx, normalizedEmail)
	if err != nil {
		return fmt.Errorf("check cooldown: %w", err)
	}
	if cooldownTTL > 0 {
		return &ThrottleError{Reason: "cooldown", RetryAfter: cooldownTTL}
	}

	if s.Config.RateLimit > 0 && s.Config.RateWindow > 0 {
		count, retryAfter, err := s.Repo.IncrementSendCount(ctx, normalizedEmail, s.Config.RateWindow)
		if err != nil {
			return fmt.Errorf("rate limit: %w", err)
		}
		if count > s.Config.RateLimit {
			return &ThrottleError{Reason: "rate_limit", RetryAfter: retryAfter}
		}
	}

	token, err := generateResetToken()
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	hash := hashToken(token, s.Config.Secret)
	if err := s.Repo.SetToken(ctx, hash, normalizedEmail, s.Config.TTL); err != nil {
		_ = s.Repo.ClearSendCount(ctx, normalizedEmail)
		return fmt.Errorf("store token: %w", err)
	}

	if err := s.Repo.SetCooldown(ctx, normalizedEmail, s.Config.Cooldown); err != nil {
		_ = s.Repo.DeleteToken(ctx, hash)
		_ = s.Repo.ClearSendCount(ctx, normalizedEmail)
		return fmt.Errorf("set cooldown: %w", err)
	}

	resetURL := buildResetURL(s.Config.URLTemplate, token)
	if err := s.Sender.SendPasswordReset(normalizedEmail, token, appName, resetURL, s.Config.TTL); err != nil {
		_ = s.Repo.DeleteToken(ctx, hash)
		_ = s.Repo.ClearCooldown(ctx, normalizedEmail)
		_ = s.Repo.ClearSendCount(ctx, normalizedEmail)
		return ErrResetDeliveryFailed
	}

	return nil
}

func (s *ServiceReset) VerifyReset(ctx context.Context, token string) (string, error) {
	if s == nil || s.Repo == nil {
		return "", ErrResetNotConfigured
	}

	cleanToken := strings.TrimSpace(token)
	if cleanToken == "" {
		return "", ErrResetInvalid
	}

	hash := hashToken(cleanToken, s.Config.Secret)
	email, err := s.Repo.ConsumeToken(ctx, hash)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrResetInvalid
		}
		return "", fmt.Errorf("get token: %w", err)
	}

	_ = s.Repo.ClearCooldown(ctx, email)
	_ = s.Repo.ClearSendCount(ctx, email)

	return email, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func generateResetToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token, secret string) string {
	h := sha256.Sum256([]byte(token + secret))
	return hex.EncodeToString(h[:])
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

var _ interfacereset.ServicePasswordResetInterface = (*ServiceReset)(nil)
