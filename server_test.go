package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"testing"
	"time"

	serviceemail "service-sender/internal/services/email"
	"service-sender/pkg/config"
	"service-sender/pkg/mailer"
)

func TestMCPEmailServicesIsolatesSenders(t *testing.T) {
	base := mailer.BrevoSender{Host: "smtp.example.com", Port: 587, From: "Default <default@example.com>"}
	cfg := config.MCPConfig{Senders: []config.EmailSenderConfig{
		{ID: "support", Email: "support@example.com", Name: "Support, 日本語"},
		{ID: "billing", Email: "billing@example.com", Name: "Billing"},
	}}
	services := mcpEmailServices(cfg, base)
	if len(services) != 2 {
		t.Fatalf("service count = %d", len(services))
	}
	for _, entry := range cfg.Senders {
		sender := services[entry.ID].(*serviceemail.ServiceEmail).Sender.(*mailer.BrevoSender)
		address, err := mail.ParseAddress(sender.From)
		if err != nil || address.Address != entry.Email || address.Name != entry.Name || sender.Host != base.Host {
			t.Fatalf("sender %s = %+v, error %v", entry.ID, sender, err)
		}
	}
	support := services["support"].(*serviceemail.ServiceEmail).Sender.(*mailer.BrevoSender)
	billing := services["billing"].(*serviceemail.ServiceEmail).Sender.(*mailer.BrevoSender)
	support.From = "Changed <changed@example.com>"
	if support == billing || strings.Contains(billing.From, "changed") || base.From != "Default <default@example.com>" {
		t.Fatal("sender instances share From state")
	}
}

func TestMCPEmailAppNameResolvesFallbackBeforeValidation(t *testing.T) {
	tests := []struct {
		name, email, otp, want string
		wantError              bool
	}{
		{name: "default", want: "Account Verification"},
		{name: "explicit", email: " Email App ", otp: "OTP App", want: "Email App"},
		{name: "empty override", otp: " OTP App ", want: "OTP App"},
		{name: "whitespace override", email: "   ", otp: "OTP App", want: "OTP App"},
		{name: "blank fallback", email: "   ", otp: "  ", want: "Account Verification"},
		{name: "unsafe empty fallback", otp: "App\r\nX-Injection: yes", wantError: true},
		{name: "unsafe whitespace fallback", email: "   ", otp: "App\r\nX-Injection: yes", wantError: true},
		{name: "unsafe primary", email: "App\r\nX-Injection: yes", otp: "Safe", wantError: true},
		{name: "unused fallback", email: "Safe", otp: "App\r\nX-Injection: yes", want: "Safe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EMAIL_APP_NAME", tt.email)
			t.Setenv("OTP_APP_NAME", tt.otp)
			name, err := mcpEmailAppName()
			if (err != nil) != tt.wantError || name != tt.want {
				t.Fatalf("name = %q, error %v", name, err)
			}
		})
	}
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func awaitHTTP(t *testing.T, addr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := client.Get("http://" + addr)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == 200 {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("server %s did not become ready: %v", addr, err)
		case <-ticker.C:
		}
	}
}

func assertPortReleased(t *testing.T, addr string) {
	t.Helper()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listener %s left open: %v", addr, err)
	}
	listener.Close()
}

func awaitTCP(t *testing.T, addr string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("server not ready")
		}
	}
}

func waitServerResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(8 * time.Second):
		t.Fatal("server lifecycle did not finish")
		return nil
	}
}

func TestServeHTTPServers(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
			api := &http.Server{Addr: unusedAddress(t), Handler: handler}
			var mcpServer *http.Server
			if enabled {
				mcpServer = &http.Server{Addr: unusedAddress(t), Handler: handler}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- serveHTTPServers(ctx, api, mcpServer) }()
			awaitHTTP(t, api.Addr)
			if enabled {
				awaitHTTP(t, mcpServer.Addr)
			}
			cancel()
			if err := waitServerResult(t, result); err != nil {
				t.Fatal(err)
			}
			assertPortReleased(t, api.Addr)
			if enabled {
				assertPortReleased(t, mcpServer.Addr)
			}
		})
	}
	t.Run("bind failure releases peer", func(t *testing.T) {
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer occupied.Close()
		api := &http.Server{Addr: unusedAddress(t)}
		mcpServer := &http.Server{Addr: occupied.Addr().String()}
		if err := serveHTTPServers(context.Background(), api, mcpServer); err == nil {
			t.Fatal("occupied MCP listener accepted")
		}
		assertPortReleased(t, api.Addr)
	})
	t.Run("server stop shuts peer down", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
		api := &http.Server{Addr: unusedAddress(t), Handler: handler}
		mcpServer := &http.Server{Addr: unusedAddress(t), Handler: handler}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- serveHTTPServers(ctx, api, mcpServer) }()
		awaitHTTP(t, api.Addr)
		awaitHTTP(t, mcpServer.Addr)
		_ = api.Close()
		if err := waitServerResult(t, result); err == nil {
			t.Fatal("unexpected server stop not reported")
		}
		assertPortReleased(t, mcpServer.Addr)
	})
	t.Run("request context follows shutdown", func(t *testing.T) {
		started, stopped := make(chan struct{}), make(chan struct{})
		api := &http.Server{Addr: unusedAddress(t), Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { close(started); <-req.Context().Done(); close(stopped) })}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- serveHTTPServers(ctx, api, nil) }()
		awaitTCP(t, api.Addr)
		client := &http.Client{Timeout: 4 * time.Second}
		defer client.CloseIdleConnections()
		clientDone := make(chan struct{})
		go func() {
			defer close(clientDone)
			response, err := client.Get("http://" + api.Addr)
			if err == nil {
				response.Body.Close()
			}
		}()
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("request not started")
		}
		cancel()
		if err := waitServerResult(t, result); err != nil {
			t.Fatal(err)
		}
		select {
		case <-stopped:
		default:
			t.Fatal("request context not canceled")
		}
		select {
		case <-clientDone:
		case <-time.After(time.Second):
			t.Fatal("request did not finish")
		}
	})
}

func TestServeHTTPServersForcesCloseOnShutdownTimeout(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	api := &http.Server{Addr: unusedAddress(t), Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}
		close(started)
		<-release
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- serveHTTPServers(ctx, api, nil) }()
	awaitTCP(t, api.Addr)
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		client := &http.Client{Timeout: 7 * time.Second}
		defer client.CloseIdleConnections()
		response, err := client.Get("http://" + api.Addr)
		if err == nil {
			response.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request not started")
	}
	cancel()
	if err := waitServerResult(t, result); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	select {
	case <-clientDone:
	case <-time.After(time.Second):
		t.Fatal("active connection not closed")
	}
	assertPortReleased(t, api.Addr)
}
