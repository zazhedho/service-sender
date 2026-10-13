package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type brevoTestRoundTripper func(*http.Request) (*http.Response, error)

func (f brevoTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestBrevoAPISenderIncludesAttachments(t *testing.T) {
	want := []byte{0x00, 0xff, 0x50, 0x44, 0x46}
	var requestBody []byte
	client := &BrevoAPIClient{apiKey: "test-key", http: &http.Client{Transport: brevoTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		var err error
		requestBody, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"messageId":"<test-id>"}`)),
			Request:    request,
		}, nil
	})}}
	sender := NewBrevoAPISender(client, "sender@example.com", "Sender")
	_, err := sender.SendTrackedEmail(context.Background(), EmailPayload{
		To: []string{"recipient@example.com"}, Subject: "CV", TextBody: "See attached",
		Attachments: []EmailAttachment{{Name: "resume.pdf", Content: want}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Attachment []struct {
			Name    string `json:"name"`
			Content []byte `json:"content"`
		} `json:"attachment"`
	}
	if err := json.Unmarshal(requestBody, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Attachment) != 1 || got.Attachment[0].Name != "resume.pdf" || !bytes.Equal(got.Attachment[0].Content, want) {
		t.Fatalf("Brevo attachment = %+v", got.Attachment)
	}
}
