package mailer

import "context"

type Sender interface {
	SendOTP(ctx context.Context, to, code, appName string) error
}
