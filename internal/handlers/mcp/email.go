package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin/binding"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"service-sender/internal/dto"
	interfaceemail "service-sender/internal/interfaces/email"
	"service-sender/pkg/config"
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
	Status    string `json:"status"`
	SenderID  string `json:"sender_id"`
	TotalSent int    `json:"total_sent"`
	Subject   string `json:"subject"`
}

type emailSendersOutput struct {
	Senders []config.EmailSenderConfig `json:"senders"`
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
		total, subject, err := svc.Send(ctx, request, strings.TrimSpace(appName))
		if err != nil {
			return nil, SendEmailOutput{}, errors.New("Email delivery failed or was canceled; outcome is unknown and may be partial. Do not retry automatically")
		}
		return nil, SendEmailOutput{Status: "smtp_accepted", SenderID: input.SenderID, TotalSent: total, Subject: subject}, nil
	})
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
