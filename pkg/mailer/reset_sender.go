package mailer

import (
	"context"
	"time"
)

type PasswordResetSender interface {
	SendPasswordReset(ctx context.Context, to, token, appName, resetURL string, ttl time.Duration) error
}
