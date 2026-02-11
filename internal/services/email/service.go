package serviceemail

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"service-sender/internal/dto"
	interfaceemail "service-sender/internal/interfaces/email"
	"service-sender/pkg/mailer"
)

var ErrEmailNotConfigured = errors.New("email sender not configured")
var ErrEmailBodyRequired = errors.New("either text_body or html_body must be provided")

type ServiceEmail struct {
	Sender mailer.EmailSender
}

func NewEmailService(sender mailer.EmailSender) *ServiceEmail {
	return &ServiceEmail{Sender: sender}
}

func (s *ServiceEmail) Send(_ context.Context, req dto.SendEmailRequest, appName string) (int, string, error) {
	if s == nil || s.Sender == nil {
		return 0, "", ErrEmailNotConfigured
	}

	subject, textBody, htmlBody, err := renderEmailContent(
		req.Subject,
		req.TextBody,
		req.HTMLBody,
		req.TemplateKey,
		req.TemplateData,
		strings.TrimSpace(appName),
	)
	if err != nil {
		return 0, "", err
	}

	to := dedupeEmails(req.To)
	if len(to) == 0 {
		return 0, "", fmt.Errorf("recipient list is empty")
	}

	payload := mailer.EmailPayload{
		Type:           req.Type,
		To:             to,
		Subject:        subject,
		TextBody:       textBody,
		HTMLBody:       htmlBody,
		ReplyTo:        strings.TrimSpace(req.ReplyTo),
		AppName:        strings.TrimSpace(appName),
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	}

	if err := s.Sender.SendEmail(payload); err != nil {
		return 0, "", err
	}
	return len(to), subject, nil
}

func dedupeEmails(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, raw := range input {
		email := strings.ToLower(strings.TrimSpace(raw))
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, email)
	}
	return out
}

var _ interfaceemail.ServiceEmailInterface = (*ServiceEmail)(nil)
