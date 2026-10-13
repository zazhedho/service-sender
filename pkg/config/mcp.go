package config

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"unicode"
)

type EmailSenderConfig struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type MCPConfig struct {
	Enabled     bool
	Addr        string
	ServerKey   string
	BrevoAPIKey string
	Senders     []EmailSenderConfig
}

func LoadMCPConfig() (MCPConfig, error) {
	cfg := MCPConfig{Addr: "127.0.0.1:8081"}
	if raw := strings.TrimSpace(os.Getenv("MCP_ENABLED")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return cfg, errors.New("MCP_ENABLED must be a boolean")
		}
		cfg.Enabled = enabled
	}
	if addr := strings.TrimSpace(os.Getenv("MCP_ADDR")); addr != "" {
		cfg.Addr = addr
	}
	if !cfg.Enabled {
		return cfg, nil
	}
	if err := validateMCPAddress(cfg.Addr); err != nil {
		return cfg, errors.New("MCP_ADDR must contain a valid host and port")
	}
	key := os.Getenv("MCP_SERVER_KEY")
	if key == "" || strings.IndexFunc(key, unicode.IsSpace) >= 0 {
		return cfg, errors.New("MCP_SERVER_KEY is required and must not contain whitespace")
	}
	cfg.ServerKey = strings.TrimSpace(key)
	cfg.BrevoAPIKey = strings.TrimSpace(os.Getenv("BREVO_API_KEY"))
	senders, err := parseMCPSenders(os.Getenv("EMAIL_VERIFIED_SENDERS"))
	if err != nil {
		return cfg, err
	}
	cfg.Senders = senders
	return cfg, nil
}

func validateMCPAddress(addr string) error {
	_, rawPort, err := net.SplitHostPort(addr)
	port, portErr := strconv.Atoi(rawPort)
	if err != nil || portErr != nil || port < 1 || port > 65535 {
		return errors.New("invalid MCP address")
	}
	return nil
}

func parseMCPSenders(raw string) ([]EmailSenderConfig, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var senders []EmailSenderConfig
	if err := decoder.Decode(&senders); err != nil {
		return nil, errors.New("EMAIL_VERIFIED_SENDERS must be a JSON array of id, email and name")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || len(senders) == 0 {
		return nil, errors.New("EMAIL_VERIFIED_SENDERS must be a single nonempty JSON array")
	}
	ids, emails := make(map[string]bool), make(map[string]bool)
	for _, sender := range senders {
		if sender.ID == "" || strings.IndexFunc(sender.ID, unicode.IsSpace) >= 0 || ids[sender.ID] {
			return nil, errors.New("EMAIL_VERIFIED_SENDERS requires unique nonempty IDs without whitespace")
		}
		address, err := mail.ParseAddress(sender.Email)
		email := strings.ToLower(sender.Email)
		if strings.ContainsAny(sender.Email+sender.Name, "\r\n") || err != nil || address.Address != sender.Email || emails[email] {
			return nil, errors.New("EMAIL_VERIFIED_SENDERS requires unique bare email addresses and safe display names")
		}
		ids[sender.ID], emails[email] = true, true
	}
	return senders, nil
}
