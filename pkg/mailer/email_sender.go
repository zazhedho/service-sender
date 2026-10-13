package mailer

import "context"

type EmailAttachment struct {
	Name    string
	Content []byte
}

type EmailPayload struct {
	Type           string
	To             []string
	Subject        string
	TextBody       string
	HTMLBody       string
	ReplyTo        string
	AppName        string
	IdempotencyKey string
	Attachments    []EmailAttachment
}

type EmailSender interface {
	SendEmail(ctx context.Context, payload EmailPayload) error
}
