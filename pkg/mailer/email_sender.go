package mailer

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
	SendEmail(payload EmailPayload) error
}
