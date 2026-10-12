package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin/binding"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"service-sender/internal/dto"
	interfaceemail "service-sender/internal/interfaces/email"
	"service-sender/pkg/config"
	"service-sender/pkg/mailer"
)

type SendEmailInput struct {
	SenderID       string   `json:"sender_id" jsonschema:"Sender ID from list_email_senders"`
	To             []string `json:"to" jsonschema:"Recipient email addresses"`
	Subject        string   `json:"subject" jsonschema:"Email subject, maximum 200 characters"`
	TextBody       string   `json:"text_body,omitempty" jsonschema:"Plain text content, maximum 10000 characters; text or HTML required"`
	HTMLBody       string   `json:"html_body,omitempty" jsonschema:"HTML content, maximum 50000 characters; text or HTML required"`
	ReplyTo        string   `json:"reply_to,omitempty" jsonschema:"Optional reply-to email address"`
	Type           string   `json:"type,omitempty" jsonschema:"campaign, info or notification; defaults to notification"`
	IdempotencyKey string   `json:"idempotency_key,omitempty" jsonschema:"Optional correlation key, maximum 100 characters; does not prevent duplicate delivery"`
}

type SendEmailOutput struct {
	Status            string                      `json:"status"`
	SenderID          string                      `json:"sender_id"`
	TotalSent         int                         `json:"total_sent"`
	Subject           string                      `json:"subject"`
	MessageID         string                      `json:"message_id,omitempty"`
	Tracking          []mailer.BrevoEmailTracking `json:"tracking,omitempty"`
	TrackingAvailable bool                        `json:"tracking_available"`
	Warning           string                      `json:"warning,omitempty"`
}

type GetEmailStatusInput struct {
	SenderID  string `json:"sender_id" jsonschema:"Sender ID used by send_email"`
	MessageID string `json:"message_id" jsonschema:"Exact message_id returned by send_email"`
}

type EmailStatusOutput struct {
	MessageID  string                 `json:"message_id"`
	Status     string                 `json:"status"`
	Recipients []EmailRecipientStatus `json:"recipients"`
}

type EmailRecipientStatus struct {
	Email         string   `json:"email"`
	LatestEvent   string   `json:"latest_event"`
	LatestAt      string   `json:"latest_at,omitempty"`
	Events        []string `json:"events"`
	DeliveredAt   string   `json:"delivered_at,omitempty"`
	OpenedAt      string   `json:"opened_at,omitempty"`
	ClickedAt     string   `json:"clicked_at,omitempty"`
	FailureReason string   `json:"failure_reason,omitempty"`
}

type emailSendersOutput struct {
	Senders []config.EmailSenderConfig `json:"senders"`
}

type trackedEmailService interface {
	SendTracked(context.Context, dto.SendEmailRequest, string) (int, string, []mailer.BrevoEmailTracking, bool, error)
	GetEmailStatus(context.Context, string) ([]mailer.BrevoEmailEvent, error)
}

func registerEmailTools(server *mcpsdk.Server, cfg config.MCPConfig, services map[string]interfaceemail.EmailSender, appName string) {
	closedWorld, additive, openWorld := false, false, true
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_email_senders",
		Description: "List configured email senders verified by the operator in Brevo. Select a sender ID from this list before sending email.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &closedWorld},
	}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, emailSendersOutput, error) {
		return nil, emailSendersOutput{Senders: cfg.Senders}, nil
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "send_email",
		Description: "Send an email using a sender ID from list_email_senders. Provide recipients, subject and text or HTML content. Success means SMTP acceptance, not inbox delivery. Idempotency keys are correlation only; do not retry automatically after an error or unknown outcome.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: &additive, OpenWorldHint: &openWorld},
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input SendEmailInput) (*mcpsdk.CallToolResult, SendEmailOutput, error) {
		if requestCtx, ok := ctx.Value(requestContextKey{}).(context.Context); ok {
			ctx = requestCtx
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if ctx.Err() != nil {
			return nil, SendEmailOutput{}, errors.New("Email request was canceled; delivery outcome is unknown. Do not retry automatically")
		}
		if err := validateEmailInput(input, appName); err != nil {
			return nil, SendEmailOutput{}, err
		}
		allowed := false
		for _, sender := range cfg.Senders {
			if sender.ID == input.SenderID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, SendEmailOutput{}, errors.New("Choose a sender_id returned by list_email_senders")
		}
		svc := services[input.SenderID]
		if svc == nil {
			return nil, SendEmailOutput{}, errors.New("Email sender is unavailable")
		}
		request := emailRequest(input)
		if input.IdempotencyKey != "" {
			pair, _ := json.Marshal([2]string{input.SenderID, input.IdempotencyKey})
			digest := sha256.Sum256(pair)
			request.IdempotencyKey = "mcp:" + hex.EncodeToString(digest[:])
		}
		var total int
		var subject string
		var tracking []mailer.BrevoEmailTracking
		var trackingAvailable bool
		var err error
		if tracked, ok := svc.(trackedEmailService); ok {
			total, subject, tracking, trackingAvailable, err = tracked.SendTracked(ctx, request, strings.TrimSpace(appName))
		} else {
			total, subject, err = svc.Send(ctx, request, strings.TrimSpace(appName))
		}
		if err != nil {
			if trackingAvailable && len(tracking) > 0 {
				return nil, SendEmailOutput{Status: "partial_or_unknown", SenderID: input.SenderID, TotalSent: total, Subject: subject, Tracking: tracking, TrackingAvailable: true, Warning: "Some recipients may have received the email. Do not retry automatically."}, nil
			}
			return nil, SendEmailOutput{}, errors.New("Email delivery failed or was canceled; outcome is unknown and may be partial. Do not retry automatically")
		}
		status := "smtp_accepted"
		if trackingAvailable {
			status = "accepted_by_brevo"
		}
		output := SendEmailOutput{Status: status, SenderID: input.SenderID, TotalSent: total, Subject: subject, Tracking: tracking, TrackingAvailable: trackingAvailable}
		if len(tracking) == 1 {
			output.MessageID = tracking[0].MessageID
		}
		return nil, output, nil
	})
	if cfg.BrevoAPIKey != "" {
		registerEmailStatusTool(server, cfg, services)
	}
}

func registerEmailStatusTool(server *mcpsdk.Server, cfg config.MCPConfig, services map[string]interfaceemail.EmailSender) {
	closedWorld := false
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_email_status",
		Description: "Look up Brevo transactional events using the exact message_id returned by send_email. Shows delivered, opened, clicked and failure events for each recipient during the last 90 days. Delivered means the receiving mail server accepted the message; an opened event is a tracking signal, not proof a person read it.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input GetEmailStatusInput) (*mcpsdk.CallToolResult, EmailStatusOutput, error) {
		if requestCtx, ok := ctx.Value(requestContextKey{}).(context.Context); ok {
			ctx = requestCtx
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if input.MessageID == "" || len(input.MessageID) > 998 || strings.ContainsAny(input.MessageID, "\r\n") {
			return nil, EmailStatusOutput{}, errors.New("Provide the exact message_id returned by send_email")
		}
		allowed := false
		for _, sender := range cfg.Senders {
			if sender.ID == input.SenderID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, EmailStatusOutput{}, errors.New("Choose a sender_id returned by list_email_senders")
		}
		tracker, ok := services[input.SenderID].(trackedEmailService)
		if !ok {
			return nil, EmailStatusOutput{}, errors.New("Brevo email tracking is unavailable for this sender")
		}
		events, err := tracker.GetEmailStatus(ctx, input.MessageID)
		if err != nil {
			return nil, EmailStatusOutput{}, errors.New("Email status lookup failed; try again later")
		}
		return nil, summarizeEmailStatus(input.MessageID, events), nil
	})
}

func summarizeEmailStatus(messageID string, events []mailer.BrevoEmailEvent) EmailStatusOutput {
	byRecipient := make(map[string]*EmailRecipientStatus)
	for _, event := range events {
		key := strings.ToLower(strings.TrimSpace(event.Email))
		if key == "" || event.Event == "" {
			continue
		}
		status := byRecipient[key]
		if status == nil {
			status = &EmailRecipientStatus{Email: event.Email, Events: []string{}}
			byRecipient[key] = status
		}
		status.LatestEvent, status.LatestAt = event.Event, event.Date
		if len(status.Events) == 0 || status.Events[len(status.Events)-1] != event.Event {
			status.Events = append(status.Events, event.Event)
		}
		switch strings.ToLower(event.Event) {
		case "delivered":
			if status.DeliveredAt == "" {
				status.DeliveredAt = event.Date
			}
		case "opened", "uniqueopened", "unique_opened", "first opening":
			if status.OpenedAt == "" {
				status.OpenedAt = event.Date
			}
		case "click":
			if status.ClickedAt == "" {
				status.ClickedAt = event.Date
			}
		case "soft_bounce", "hard_bounce", "blocked", "spam", "invalid", "invalid_email", "error":
			status.FailureReason = event.Reason
		}
	}
	output := EmailStatusOutput{MessageID: messageID, Status: "events_found"}
	if len(byRecipient) == 0 {
		output.Status = "no_events_yet"
		return output
	}
	for _, status := range byRecipient {
		output.Recipients = append(output.Recipients, *status)
	}
	sort.Slice(output.Recipients, func(i, j int) bool { return output.Recipients[i].Email < output.Recipients[j].Email })
	return output
}

func emailRequest(input SendEmailInput) dto.SendEmailRequest {
	emailType := input.Type
	if emailType == "" {
		emailType = "notification"
	}
	return dto.SendEmailRequest{
		Type: emailType, To: input.To, Subject: input.Subject, TextBody: input.TextBody,
		HTMLBody: input.HTMLBody, ReplyTo: input.ReplyTo, IdempotencyKey: input.IdempotencyKey,
	}
}

func validateEmailInput(input SendEmailInput, appName string) error {
	if strings.ContainsAny(appName, "\r\n") {
		return errors.New("Email application configuration is invalid")
	}
	headers := []string{input.Subject, input.ReplyTo, input.IdempotencyKey}
	headers = append(headers, input.To...)
	for _, header := range headers {
		if strings.ContainsAny(header, "\r\n") {
			return errors.New("Email headers must not contain line breaks")
		}
	}
	if strings.TrimSpace(input.Subject) == "" {
		return errors.New("subject is required")
	}
	if strings.TrimSpace(input.TextBody) == "" && strings.TrimSpace(input.HTMLBody) == "" {
		return errors.New("text_body or html_body is required")
	}
	if err := binding.Validator.ValidateStruct(emailRequest(input)); err != nil {
		return errors.New("Invalid email input; check recipients, type and field length limits")
	}
	return nil
}
