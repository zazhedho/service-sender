package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	interfaceemail "service-sender/internal/interfaces/email"
	serviceemail "service-sender/internal/services/email"
	"service-sender/pkg/config"
	"service-sender/pkg/mailer"
)

func mcpEmailAppName() (string, error) {
	name := strings.TrimSpace(os.Getenv("EMAIL_APP_NAME"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("OTP_APP_NAME"))
	}
	if name == "" {
		name = "Account Verification"
	}
	if strings.ContainsAny(name, "\r\n") {
		return "", errors.New("email application name must not contain line breaks")
	}
	return name, nil
}

func mcpEmailServices(cfg config.MCPConfig, sender mailer.BrevoSender) map[string]interfaceemail.EmailSender {
	services := make(map[string]interfaceemail.EmailSender, len(cfg.Senders))
	var brevoAPI *mailer.BrevoAPIClient
	if cfg.BrevoAPIKey != "" {
		brevoAPI = mailer.NewBrevoAPIClient(cfg.BrevoAPIKey)
	}
	for _, entry := range cfg.Senders {
		selectedSender := sender
		selectedSender.From = (&mail.Address{Name: entry.Name, Address: entry.Email}).String()
		var emailSender mailer.EmailSender = &selectedSender
		if brevoAPI != nil {
			emailSender = mailer.NewBrevoAPISender(brevoAPI, entry.Email, entry.Name)
		}
		services[entry.ID] = serviceemail.NewEmailService(emailSender)
	}
	return services
}

func serveHTTPServers(ctx context.Context, api *http.Server, mcpServer *http.Server) error {
	if api == nil {
		return errors.New("HTTP server is required")
	}
	lifetime, stop := context.WithCancel(ctx)
	defer stop()
	servers := []*http.Server{api}
	if mcpServer != nil {
		servers = append(servers, mcpServer)
	}
	listeners := make([]net.Listener, 0, len(servers))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	for _, server := range servers {
		listener, err := (&net.ListenConfig{}).Listen(lifetime, "tcp", server.Addr)
		if err != nil {
			return fmt.Errorf("bind HTTP listener: %w", err)
		}
		listeners = append(listeners, listener)
		server.BaseContext = func(net.Listener) context.Context { return lifetime }
	}
	results := make(chan error, len(servers))
	for index, server := range servers {
		go func() { results <- server.Serve(listeners[index]) }()
	}
	remaining := len(servers)
	var result error
	select {
	case <-ctx.Done():
	case err := <-results:
		remaining--
		if ctx.Err() == nil {
			result = fmt.Errorf("HTTP server stopped unexpectedly: %w", err)
		}
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	shutdownResults := make(chan error, len(servers))
	for _, server := range servers {
		go func() {
			err := server.Shutdown(shutdown)
			if err != nil {
				_ = server.Close()
			}
			shutdownResults <- err
		}()
	}
	for range servers {
		result = errors.Join(result, <-shutdownResults)
	}
	for range remaining {
		if err := <-results; err != nil && !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}
	return result
}
