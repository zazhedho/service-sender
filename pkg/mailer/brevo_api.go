package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const brevoAPIBaseURL = "https://api.brevo.com/v3"

type BrevoAPIClient struct {
	apiKey string
	http   *http.Client
}

type BrevoEmailEvent struct {
	Email  string `json:"email"`
	Event  string `json:"event"`
	Date   string `json:"date"`
	Reason string `json:"reason,omitempty"`
}

type BrevoEmailTracking struct {
	Recipient string `json:"recipient"`
	MessageID string `json:"message_id"`
}

type TrackedEmailSender interface {
	SendTrackedEmail(context.Context, EmailPayload) ([]BrevoEmailTracking, error)
}

type EmailStatusTracker interface {
	GetEmailStatus(context.Context, string) ([]BrevoEmailEvent, error)
}

type BrevoAPISender struct {
	Client    *BrevoAPIClient
	FromEmail string
	FromName  string
}

func NewBrevoAPIClient(apiKey string) *BrevoAPIClient {
	return &BrevoAPIClient{apiKey: strings.TrimSpace(apiKey), http: &http.Client{Timeout: 30 * time.Second}}
}

func NewBrevoAPISender(client *BrevoAPIClient, fromEmail, fromName string) *BrevoAPISender {
	return &BrevoAPISender{Client: client, FromEmail: fromEmail, FromName: fromName}
}

func (s *BrevoAPISender) SendEmail(ctx context.Context, payload EmailPayload) error {
	_, err := s.SendTrackedEmail(ctx, payload)
	return err
}

func (s *BrevoAPISender) SendTrackedEmail(ctx context.Context, payload EmailPayload) ([]BrevoEmailTracking, error) {
	if s == nil || s.Client == nil || s.Client.apiKey == "" {
		return nil, errors.New("Brevo API is not configured")
	}
	tracking := make([]BrevoEmailTracking, 0, len(payload.To))
	for _, recipient := range payload.To {
		message := payload
		message.To = []string{recipient}
		messageID, err := s.Client.sendEmail(ctx, s.FromEmail, s.FromName, message)
		if err != nil {
			return tracking, err
		}
		tracking = append(tracking, BrevoEmailTracking{Recipient: recipient, MessageID: messageID})
	}
	return tracking, nil
}

func (c *BrevoAPIClient) sendEmail(ctx context.Context, fromEmail, fromName string, payload EmailPayload) (string, error) {
	textBody, htmlBody := payload.TextBody, payload.HTMLBody
	if strings.TrimSpace(textBody) == "" && strings.TrimSpace(htmlBody) != "" {
		textBody = "Please open this email in HTML mode."
	}
	if strings.TrimSpace(htmlBody) == "" {
		htmlBody = "<p>" + html.EscapeString(textBody) + "</p>"
	}
	requestBody := struct {
		Sender struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"sender"`
		To []struct {
			Email string `json:"email"`
		} `json:"to"`
		Subject     string `json:"subject"`
		TextContent string `json:"textContent,omitempty"`
		HTMLContent string `json:"htmlContent,omitempty"`
		ReplyTo     *struct {
			Email string `json:"email"`
		} `json:"replyTo,omitempty"`
		Headers    map[string]string `json:"headers,omitempty"`
		Attachment []struct {
			Name    string `json:"name"`
			Content []byte `json:"content"`
		} `json:"attachment,omitempty"`
	}{}
	requestBody.Sender.Email = fromEmail
	requestBody.Sender.Name = fromName
	requestBody.Subject = payload.Subject
	requestBody.TextContent = textBody
	requestBody.HTMLContent = htmlBody
	for _, recipient := range payload.To {
		requestBody.To = append(requestBody.To, struct {
			Email string `json:"email"`
		}{Email: recipient})
	}
	if payload.ReplyTo != "" {
		requestBody.ReplyTo = &struct {
			Email string `json:"email"`
		}{Email: payload.ReplyTo}
	}
	for _, attachment := range payload.Attachments {
		requestBody.Attachment = append(requestBody.Attachment, struct {
			Name    string `json:"name"`
			Content []byte `json:"content"`
		}{Name: attachment.Name, Content: attachment.Content})
	}
	if payload.AppName != "" || payload.IdempotencyKey != "" {
		requestBody.Headers = make(map[string]string, 2)
		if payload.AppName != "" {
			requestBody.Headers["X-App-Name"] = payload.AppName
		}
		if payload.IdempotencyKey != "" {
			requestBody.Headers["X-Idempotency-Key"] = payload.IdempotencyKey
		}
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", errors.New("could not encode email request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, brevoAPIBaseURL+"/smtp/email", bytes.NewReader(body))
	if err != nil {
		return "", errors.New("could not create Brevo request")
	}
	request.Header.Set("api-key", c.apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return "", errors.New("Brevo email request failed; outcome may be unknown. Do not retry automatically")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Brevo email request failed with HTTP %d", response.StatusCode)
	}
	var result struct {
		MessageID string `json:"messageId"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil || strings.TrimSpace(result.MessageID) == "" {
		return "", errors.New("Brevo accepted the email but returned no tracking ID; outcome is unknown. Do not retry automatically")
	}
	return result.MessageID, nil
}

func (s *BrevoAPISender) GetEmailStatus(ctx context.Context, messageID string) ([]BrevoEmailEvent, error) {
	if s == nil || s.Client == nil || s.Client.apiKey == "" {
		return nil, errors.New("Brevo API is not configured")
	}
	query := url.Values{}
	query.Set("messageId", messageID)
	query.Set("days", "90")
	query.Set("limit", "5000")
	query.Set("sort", "asc")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, brevoAPIBaseURL+"/smtp/statistics/events?"+query.Encode(), nil)
	if err != nil {
		return nil, errors.New("could not create Brevo status request")
	}
	request.Header.Set("api-key", s.Client.apiKey)
	request.Header.Set("Accept", "application/json")
	response, err := s.Client.http.Do(request)
	if err != nil {
		return nil, errors.New("Brevo status request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Brevo status request failed with HTTP %d", response.StatusCode)
	}
	var result struct {
		Events []BrevoEmailEvent `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return nil, errors.New("could not decode Brevo status response")
	}
	return result.Events, nil
}

var _ EmailSender = (*BrevoAPISender)(nil)
var _ TrackedEmailSender = (*BrevoAPISender)(nil)
var _ EmailStatusTracker = (*BrevoAPISender)(nil)
