package serviceemail

import (
	"context"
	"errors"
	"testing"

	"service-sender/internal/dto"
	"service-sender/pkg/mailer"
)

type recordingSender struct {
	ctx     context.Context
	payload mailer.EmailPayload
	calls   int
	err     error
}

func (s *recordingSender) SendEmail(ctx context.Context, payload mailer.EmailPayload) error {
	s.ctx, s.payload = ctx, payload
	s.calls++
	return s.err
}

func TestEmailServiceForwardsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := &recordingSender{}
	svc := NewEmailService(sender)
	total, subject, err := svc.Send(ctx, dto.SendEmailRequest{
		Type: "notification", To: []string{"A@example.com", "a@example.com", "b@example.com"},
		Subject: " Hello ", TextBody: " Text ", IdempotencyKey: "rest-key",
	}, " App ")
	if err != nil || total != 2 || subject != "Hello" {
		t.Fatalf("send = %d, %q, %v", total, subject, err)
	}
	if sender.ctx != ctx || sender.calls != 1 || sender.payload.To[0] != "a@example.com" || sender.payload.AppName != "App" || sender.payload.IdempotencyKey != "rest-key" {
		t.Fatalf("payload = %+v, calls %d", sender.payload, sender.calls)
	}
}

func TestEmailServiceRejectsMissingContent(t *testing.T) {
	sender := &recordingSender{}
	_, _, err := NewEmailService(sender).Send(context.Background(), dto.SendEmailRequest{Subject: "Hello", To: []string{"a@example.com"}}, "App")
	if !errors.Is(err, ErrEmailBodyRequired) || sender.calls != 0 {
		t.Fatalf("error = %v, calls %d", err, sender.calls)
	}
	_, _, err = NewEmailService(nil).Send(context.Background(), dto.SendEmailRequest{}, "App")
	if !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("error = %v", err)
	}
}

func TestEmailServiceForwardsSenderError(t *testing.T) {
	want := errors.New("SMTP unavailable")
	sender := &recordingSender{err: want}
	_, _, err := NewEmailService(sender).Send(context.Background(), dto.SendEmailRequest{To: []string{"a@example.com"}, Subject: "Hello", TextBody: "Text"}, "App")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
