package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"service-sender/internal/dto"
	interfaceemail "service-sender/internal/interfaces/email"
	"service-sender/pkg/config"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type testEmailService struct {
	mu               sync.Mutex
	requests         []dto.SendEmailRequest
	apps             []string
	err              error
	started, stopped chan struct{}
}

func (s *testEmailService) Send(ctx context.Context, req dto.SendEmailRequest, appName string) (int, string, error) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.apps = append(s.apps, appName)
	s.mu.Unlock()
	if s.started != nil {
		close(s.started)
		<-ctx.Done()
		close(s.stopped)
		return 0, "", ctx.Err()
	}
	if s.err != nil {
		return 0, "", s.err
	}
	return len(req.To), strings.TrimSpace(req.Subject), nil
}

func (s *testEmailService) snapshot() ([]dto.SendEmailRequest, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]dto.SendEmailRequest(nil), s.requests...), append([]string(nil), s.apps...)
}

func testMCPConfig() config.MCPConfig {
	return config.MCPConfig{Enabled: true, ServerKey: "test-key", Senders: []config.EmailSenderConfig{
		{ID: "support", Email: "support@example.com", Name: "Support"},
		{ID: "billing", Email: "billing@example.com", Name: "Billing"},
	}}
}

type bearerTransport struct{ base http.RoundTripper }

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer test-key")
	return b.base.RoundTrip(clone)
}

func newMCPSession(t *testing.T, services map[string]interfaceemail.EmailSender, appName string) *mcpsdk.ClientSession {
	t.Helper()
	server := httptest.NewServer(NewHTTPHandler(testMCPConfig(), services, appName))
	t.Cleanup(server.Close)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{base: server.Client().Transport}},
		MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func toolCall(t *testing.T, session *mcpsdk.ClientSession, name string, args any) *mcpsdk.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMCPDiscoveryAndSendEmail(t *testing.T) {
	support, billing := &testEmailService{}, &testEmailService{}
	session := newMCPSession(t, map[string]interfaceemail.EmailSender{"support": support, "billing": billing}, "Sender Service")
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("tool count = %d", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		switch tool.Name {
		case "list_email_senders":
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Fatal("sender list is not read-only")
			}
		case "send_email":
			if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint {
				t.Fatal("send annotations incorrectly allow automatic retry")
			}
		default:
			t.Fatalf("unexpected tool %s", tool.Name)
		}
	}
	list := toolCall(t, session, "list_email_senders", map[string]any{})
	var senders struct {
		Senders []config.EmailSenderConfig `json:"senders"`
	}
	encoded, _ := json.Marshal(list.StructuredContent)
	if list.IsError || json.Unmarshal(encoded, &senders) != nil || len(senders.Senders) != 2 || senders.Senders[0].ID != "support" || senders.Senders[1].Email != "billing@example.com" {
		t.Fatalf("senders = %s", encoded)
	}
	result := toolCall(t, session, "send_email", map[string]any{"sender_id": "billing", "to": []string{"a@example.com"}, "subject": " Invoice ", "text_body": "Hello", "reply_to": "reply@example.com"})
	var output SendEmailOutput
	encoded, _ = json.Marshal(result.StructuredContent)
	if result.IsError || json.Unmarshal(encoded, &output) != nil || output.Status != "smtp_accepted" || output.SenderID != "billing" || output.TotalSent != 1 || output.Subject != "Invoice" {
		t.Fatalf("send output = %s", encoded)
	}
	billingRequests, apps := billing.snapshot()
	supportRequests, _ := support.snapshot()
	if len(supportRequests) != 0 || len(billingRequests) != 1 || billingRequests[0].Type != "notification" || billingRequests[0].ReplyTo != "reply@example.com" || apps[0] != "Sender Service" {
		t.Fatalf("wrong sender/payload: %+v / %+v", supportRequests, billingRequests)
	}
}

func TestMCPRejectsInvalidInput(t *testing.T) {
	service := &testEmailService{}
	session := newMCPSession(t, map[string]interfaceemail.EmailSender{"support": service, "arbitrary": service}, "App")
	tests := []struct {
		name, field string
		value       any
	}{
		{"unknown sender", "sender_id", "arbitrary"},
		{"empty subject", "subject", " "},
		{"empty body", "text_body", "\t\n"},
		{"invalid recipient", "to", []string{"not-email"}},
		{"empty recipients", "to", []string{}},
		{"bad reply", "reply_to", "not-email"},
		{"invalid type", "type", "unknown"},
		{"long subject", "subject", strings.Repeat("s", 201)},
		{"long text", "text_body", strings.Repeat("s", 10001)},
		{"long html", "html_body", strings.Repeat("s", 50001)},
		{"long key", "idempotency_key", strings.Repeat("s", 101)},
		{"subject injection", "subject", "Hi\r\nBcc: other@example.com"},
		{"reply injection", "reply_to", "a@example.com\nBcc: other@example.com"},
		{"key injection", "idempotency_key", "key\nBcc: other@example.com"},
		{"recipient injection", "to", []string{"a@example.com\r\nBcc: other@example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"sender_id": "support", "to": []string{"a@example.com"}, "subject": "Hello", "text_body": "Body"}
			args[tt.field] = tt.value
			result := toolCall(t, session, "send_email", args)
			if !result.IsError {
				t.Fatal("invalid input accepted")
			}
		})
	}
	requests, _ := service.snapshot()
	if len(requests) != 0 {
		t.Fatalf("invalid input sent %d emails", len(requests))
	}
}

func TestMCPSafeErrors(t *testing.T) {
	for _, app := range []string{"App", "App\r\nInjected: yes"} {
		t.Run(app, func(t *testing.T) {
			service := &testEmailService{err: errors.New("accepted first recipient, then failed: credential=test-secret")}
			session := newMCPSession(t, map[string]interfaceemail.EmailSender{"support": service}, app)
			result := toolCall(t, session, "send_email", map[string]any{"sender_id": "support", "to": []string{"a@example.com", "b@example.com"}, "subject": "Hello", "text_body": "Body"})
			data, _ := json.Marshal(result)
			if !result.IsError || strings.Contains(string(data), "test-secret") || strings.Contains(string(data), "Injected") {
				t.Fatalf("unsafe result: %s", data)
			}
			if app == "App" && (!strings.Contains(string(data), "unknown") || !strings.Contains(string(data), "retry")) {
				t.Fatalf("ambiguous SMTP outcome not explained: %s", data)
			}
			if app != "App" {
				calls, _ := service.snapshot()
				if len(calls) != 0 {
					t.Fatal("unsafe app name reached sender")
				}
			}
		})
	}
	t.Run("unconfigured sender still lists", func(t *testing.T) {
		session := newMCPSession(t, nil, "App")
		if toolCall(t, session, "list_email_senders", map[string]any{}).IsError {
			t.Fatal("list required SMTP")
		}
		if !toolCall(t, session, "send_email", map[string]any{"sender_id": "support", "to": []string{"a@example.com"}, "subject": "Hello", "text_body": "Body"}).IsError {
			t.Fatal("missing sender accepted")
		}
	})
}

func TestMCPIdempotencyHeader(t *testing.T) {
	service := &testEmailService{}
	session := newMCPSession(t, map[string]interfaceemail.EmailSender{"support": service, "billing": service}, "App")
	for _, pair := range [][2]string{{"support", "key"}, {"support", "key"}, {"billing", "key"}, {"support", ""}} {
		result := toolCall(t, session, "send_email", map[string]any{"sender_id": pair[0], "to": []string{"a@example.com"}, "subject": "Hello", "text_body": "Body", "idempotency_key": pair[1]})
		if result.IsError {
			t.Fatal("valid key rejected")
		}
	}
	requests, _ := service.snapshot()
	if len(requests) != 4 || requests[0].IdempotencyKey != requests[1].IdempotencyKey || requests[0].IdempotencyKey == requests[2].IdempotencyKey || !strings.HasPrefix(requests[0].IdempotencyKey, "mcp:") || len(requests[0].IdempotencyKey) > 100 || requests[3].IdempotencyKey != "" {
		t.Fatalf("header namespace incorrect: %+v", requests)
	}
}

const initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

func TestMCPAuthenticationAndLimits(t *testing.T) {
	handler := NewHTTPHandler(testMCPConfig(), nil, "App")
	tests := []struct {
		name               string
		auth               []string
		body, path, origin string
		want               int
	}{
		{name: "missing", want: 401},
		{name: "wrong", auth: []string{"Bearer wrong"}, want: 401},
		{name: "duplicate", auth: []string{"Bearer test-key", "Bearer test-key"}, want: 401},
		{name: "valid", auth: []string{"Bearer test-key"}, want: 200},
		{name: "case insensitive scheme", auth: []string{"bearer test-key"}, want: 200},
		{name: "oversized", auth: []string{"Bearer test-key"}, body: strings.Repeat(" ", 1<<20) + initializeRequest, want: 413},
		{name: "other path", auth: []string{"Bearer test-key"}, path: "/other", want: 404},
		{name: "cross origin", auth: []string{"Bearer test-key"}, origin: "https://attacker.example", want: 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, path := tt.body, tt.path
			if body == "" {
				body = initializeRequest
			}
			if path == "" {
				path = "/mcp"
			}
			req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			for _, auth := range tt.auth {
				req.Header.Add("Authorization", auth)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d; body %s", response.Code, tt.want, response.Body.String())
			}
		})
	}
}

func TestMCPLegacyRequestCancellation(t *testing.T) {
	service := &testEmailService{started: make(chan struct{}), stopped: make(chan struct{})}
	server := httptest.NewServer(NewHTTPHandler(testMCPConfig(), map[string]interfaceemail.EmailSender{"support": service}, "App"))
	defer server.Close()
	body := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_email","arguments":{"sender_id":"support","to":["a@example.com"],"subject":"Hello","text_body":"Body"}}}`
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	result := make(chan error, 1)
	go func() {
		resp, err := server.Client().Do(req)
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		result <- err
	}()
	select {
	case <-service.started:
	case <-time.After(3 * time.Second):
		t.Fatal("tool did not start")
	}
	cancel()
	select {
	case <-service.stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP cancellation did not reach email service")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client did not stop")
	}
}
