package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin/binding"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"service-sender/internal/dto"
	interfaceemail "service-sender/internal/interfaces/email"
	"service-sender/pkg/config"
	"service-sender/pkg/mailer"
)

type SendEmailInput struct {
	SenderID       string                     `json:"sender_id" jsonschema:"Sender ID from list_email_senders"`
	To             []string                   `json:"to" jsonschema:"Recipient email addresses"`
	Subject        string                     `json:"subject" jsonschema:"Email subject, maximum 200 characters"`
	TextBody       string                     `json:"text_body,omitempty" jsonschema:"Plain text content, maximum 10000 characters; text or HTML required"`
	HTMLBody       string                     `json:"html_body,omitempty" jsonschema:"HTML content, maximum 50000 characters; text or HTML required"`
	ReplyTo        string                     `json:"reply_to,omitempty" jsonschema:"Optional reply-to email address"`
	Type           string                     `json:"type,omitempty" jsonschema:"campaign, info or notification; defaults to notification"`
	IdempotencyKey string                     `json:"idempotency_key,omitempty" jsonschema:"Optional correlation key, maximum 100 characters; does not prevent duplicate delivery"`
	Attachments    []SendEmailAttachmentInput `json:"attachments,omitempty" jsonschema:"Optional files; Base64 content, at most 10 MiB decoded total"`
}

type SendEmailAttachmentInput struct {
	Name          string `json:"name" jsonschema:"Filename without a path"`
	ContentBase64 string `json:"content_base64" jsonschema:"File bytes encoded as standard Base64"`
}

const maxEmailAttachmentBytes = 10 << 20

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
	}, sendEmailToolHandler(cfg, services, appName))
	if cfg.BrevoAPIKey != "" {
		registerEmailStatusTool(server, cfg, services)
	}
}

func sendEmailToolHandler(cfg config.MCPConfig, services map[string]interfaceemail.EmailSender, appName string) func(context.Context, *mcpsdk.CallToolRequest, SendEmailInput) (*mcpsdk.CallToolResult, SendEmailOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, input SendEmailInput) (*mcpsdk.CallToolResult, SendEmailOutput, error) {
		ctx, cancel := context.WithTimeout(emailRequestContext(ctx), 30*time.Second)
		defer cancel()
		output, err := sendEmail(ctx, cfg, services, appName, input)
		return nil, output, err
	}
}

func emailRequestContext(ctx context.Context) context.Context {
	if requestCtx, ok := ctx.Value(requestContextKey{}).(context.Context); ok {
		return requestCtx
	}
	return ctx
}

type emailSendResult struct {
	total             int
	subject           string
	tracking          []mailer.BrevoEmailTracking
	trackingAvailable bool
}

func sendEmail(ctx context.Context, cfg config.MCPConfig, services map[string]interfaceemail.EmailSender, appName string, input SendEmailInput) (SendEmailOutput, error) {
	if ctx.Err() != nil {
		return SendEmailOutput{}, errors.New("Email request was canceled; delivery outcome is unknown. Do not retry automatically")
	}
	if err := validateEmailInput(input, appName); err != nil {
		return SendEmailOutput{}, err
	}
	svc, err := configuredEmailSender(cfg, services, input.SenderID)
	if err != nil {
		return SendEmailOutput{}, err
	}
	request, err := prepareEmailRequest(input)
	if err != nil {
		return SendEmailOutput{}, err
	}
	result, err := sendEmailWithTracking(ctx, svc, request, appName)
	return emailSendOutput(input.SenderID, result, err)
}

func configuredEmailSender(cfg config.MCPConfig, services map[string]interfaceemail.EmailSender, senderID string) (interfaceemail.EmailSender, error) {
	if !emailSenderAllowed(cfg, senderID) {
		return nil, errors.New("Choose a sender_id returned by list_email_senders")
	}
	svc := services[senderID]
	if svc == nil {
		return nil, errors.New("Email sender is unavailable")
	}
	return svc, nil
}

func emailSenderAllowed(cfg config.MCPConfig, senderID string) bool {
	for _, sender := range cfg.Senders {
		if sender.ID == senderID {
			return true
		}
	}
	return false
}

func prepareEmailRequest(input SendEmailInput) (dto.SendEmailRequest, error) {
	attachments, err := decodeEmailAttachments(input.Attachments)
	if err != nil {
		return dto.SendEmailRequest{}, err
	}
	request := emailRequest(input)
	request.Attachments = attachments
	if input.IdempotencyKey != "" {
		pair, _ := json.Marshal([2]string{input.SenderID, input.IdempotencyKey})
		digest := sha256.Sum256(pair)
		request.IdempotencyKey = "mcp:" + hex.EncodeToString(digest[:])
	}
	return request, nil
}

func sendEmailWithTracking(ctx context.Context, svc interfaceemail.EmailSender, request dto.SendEmailRequest, appName string) (emailSendResult, error) {
	if tracked, ok := svc.(trackedEmailService); ok {
		total, subject, tracking, available, err := tracked.SendTracked(ctx, request, strings.TrimSpace(appName))
		return emailSendResult{total, subject, tracking, available}, err
	}
	total, subject, err := svc.Send(ctx, request, strings.TrimSpace(appName))
	return emailSendResult{total: total, subject: subject}, err
}

func emailSendOutput(senderID string, result emailSendResult, err error) (SendEmailOutput, error) {
	if err != nil {
		if result.trackingAvailable && len(result.tracking) > 0 {
			return SendEmailOutput{
				Status: "partial_or_unknown", SenderID: senderID, TotalSent: result.total, Subject: result.subject,
				Tracking: result.tracking, TrackingAvailable: true,
				Warning: "Some recipients may have received the email. Do not retry automatically.",
			}, nil
		}
		return SendEmailOutput{}, errors.New("Email delivery failed or was canceled; outcome is unknown and may be partial. Do not retry automatically")
	}
	status := "smtp_accepted"
	if result.trackingAvailable {
		status = "accepted_by_brevo"
	}
	output := SendEmailOutput{
		Status: status, SenderID: senderID, TotalSent: result.total, Subject: result.subject,
		Tracking: result.tracking, TrackingAvailable: result.trackingAvailable,
	}
	if len(result.tracking) == 1 {
		output.MessageID = result.tracking[0].MessageID
	}
	return output, nil
}

func decodeEmailAttachments(inputs []SendEmailAttachmentInput) ([]dto.EmailAttachment, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	attachments := make([]dto.EmailAttachment, 0, len(inputs))
	totalBytes := 0
	for _, input := range inputs {
		if !utf8.ValidString(input.Name) || strings.TrimSpace(input.Name) == "" || input.Name == "." || input.Name == ".." || len(input.Name) > 255 ||
			strings.ContainsAny(input.Name, `/\`) || strings.IndexFunc(input.Name, unicode.IsControl) >= 0 {
			return nil, errors.New("Invalid attachment; check its filename and Base64 content")
		}
		content, err := base64.StdEncoding.Strict().DecodeString(input.ContentBase64)
		if err != nil || len(content) == 0 {
			return nil, errors.New("Invalid attachment; check its filename and Base64 content")
		}
		if len(content) > maxEmailAttachmentBytes-totalBytes {
			return nil, errors.New("Attachments exceed the 10 MiB total limit")
		}
		totalBytes += len(content)
		attachments = append(attachments, dto.EmailAttachment{Name: input.Name, Content: content})
	}
	return attachments, nil
}

func registerEmailStatusTool(server *mcpsdk.Server, cfg config.MCPConfig, services map[string]interfaceemail.EmailSender) {
	closedWorld := false
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_email_status",
		Description: "Look up Brevo transactional events using the exact message_id returned by send_email. Shows delivered, opened, clicked and failure events for each recipient during the last 90 days. Delivered means the receiving mail server accepted the message; an opened event is a tracking signal, not proof a person read it.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input GetEmailStatusInput) (*mcpsdk.CallToolResult, EmailStatusOutput, error) {
		ctx, cancel := context.WithTimeout(emailRequestContext(ctx), 30*time.Second)
		defer cancel()
		output, err := getEmailStatus(ctx, cfg, services, input)
		return nil, output, err
	})
}

func getEmailStatus(ctx context.Context, cfg config.MCPConfig, services map[string]interfaceemail.EmailSender, input GetEmailStatusInput) (EmailStatusOutput, error) {
	if input.MessageID == "" || len(input.MessageID) > 998 || strings.ContainsAny(input.MessageID, "\r\n") {
		return EmailStatusOutput{}, errors.New("Provide the exact message_id returned by send_email")
	}
	if !emailSenderAllowed(cfg, input.SenderID) {
		return EmailStatusOutput{}, errors.New("Choose a sender_id returned by list_email_senders")
	}
	tracker, ok := services[input.SenderID].(trackedEmailService)
	if !ok {
		return EmailStatusOutput{}, errors.New("Brevo email tracking is unavailable for this sender")
	}
	events, err := tracker.GetEmailStatus(ctx, input.MessageID)
	if err != nil {
		return EmailStatusOutput{}, errors.New("Email status lookup failed; try again later")
	}
	return summarizeEmailStatus(input.MessageID, events), nil
}

func summarizeEmailStatus(messageID string, events []mailer.BrevoEmailEvent) EmailStatusOutput {
	byRecipient := make(map[string]*EmailRecipientStatus)
	for _, event := range events {
		status := emailStatusForEvent(byRecipient, event)
		if status != nil {
			applyEmailEvent(status, event)
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

func emailStatusForEvent(byRecipient map[string]*EmailRecipientStatus, event mailer.BrevoEmailEvent) *EmailRecipientStatus {
	key := strings.ToLower(strings.TrimSpace(event.Email))
	if key == "" || event.Event == "" {
		return nil
	}
	status := byRecipient[key]
	if status == nil {
		status = &EmailRecipientStatus{Email: event.Email, Events: []string{}}
		byRecipient[key] = status
	}
	return status
}

func applyEmailEvent(status *EmailRecipientStatus, event mailer.BrevoEmailEvent) {
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
