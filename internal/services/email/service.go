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

func (s *ServiceEmail) Send(ctx context.Context, req dto.SendEmailRequest, appName string) (int, string, error) {
	total, subject, _, _, err := s.send(ctx, req, appName)
	return total, subject, err
}

func (s *ServiceEmail) SendTracked(ctx context.Context, req dto.SendEmailRequest, appName string) (int, string, []mailer.BrevoEmailTracking, bool, error) {
	return s.send(ctx, req, appName)
}

func (s *ServiceEmail) GetEmailStatus(ctx context.Context, messageID string) ([]mailer.BrevoEmailEvent, error) {
	tracker, ok := s.Sender.(mailer.EmailStatusTracker)
	if !ok {
		return nil, errors.New("email status tracking is not configured")
	}
	return tracker.GetEmailStatus(ctx, messageID)
}

func (s *ServiceEmail) send(ctx context.Context, req dto.SendEmailRequest, appName string) (int, string, []mailer.BrevoEmailTracking, bool, error) {
	if s == nil || s.Sender == nil {
		return 0, "", nil, false, ErrEmailNotConfigured
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
		return 0, "", nil, false, err
	}

	to := dedupeEmails(req.To)
	if len(to) == 0 {
		return 0, "", nil, false, fmt.Errorf("recipient list is empty")
	}

	attachments := make([]mailer.EmailAttachment, 0, len(req.Attachments))
	for _, attachment := range req.Attachments {
		attachments = append(attachments, mailer.EmailAttachment{Name: attachment.Name, Content: attachment.Content})
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
		Attachments:    attachments,
	}

	if sender, ok := s.Sender.(mailer.TrackedEmailSender); ok {
		tracking, err := sender.SendTrackedEmail(ctx, payload)
		if err != nil {
			return len(tracking), subject, tracking, true, err
		}
		return len(tracking), subject, tracking, true, nil
	}
	if err := s.Sender.SendEmail(ctx, payload); err != nil {
		return 0, "", nil, false, err
	}
	return len(to), subject, nil, false, nil
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

var _ interfaceemail.EmailSender = (*ServiceEmail)(nil)
