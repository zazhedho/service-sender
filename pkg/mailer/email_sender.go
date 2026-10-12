package mailer

import "context"

type EmailPayload struct {
	Type           string
	To             []string
	Subject        string
	TextBody       string
	HTMLBody       string
	ReplyTo        string
	AppName        string
	IdempotencyKey string
}

type EmailSender interface {
	SendEmail(ctx context.Context, payload EmailPayload) error
}
