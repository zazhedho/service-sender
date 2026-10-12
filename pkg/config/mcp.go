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
	Enabled   bool
	Addr      string
	ServerKey string
	Senders   []EmailSenderConfig
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
	_, rawPort, err := net.SplitHostPort(cfg.Addr)
	port, portErr := strconv.Atoi(rawPort)
	if err != nil || portErr != nil || port < 1 || port > 65535 {
		return cfg, errors.New("MCP_ADDR must contain a valid host and port")
	}
	key := os.Getenv("MCP_SERVER_KEY")
	if key == "" || strings.IndexFunc(key, unicode.IsSpace) >= 0 {
		return cfg, errors.New("MCP_SERVER_KEY is required and must not contain whitespace")
	}
	cfg.ServerKey = strings.TrimSpace(key)
	decoder := json.NewDecoder(strings.NewReader(os.Getenv("EMAIL_VERIFIED_SENDERS")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg.Senders); err != nil {
		return cfg, errors.New("EMAIL_VERIFIED_SENDERS must be a JSON array of id, email and name")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || len(cfg.Senders) == 0 {
		return cfg, errors.New("EMAIL_VERIFIED_SENDERS must be a single nonempty JSON array")
	}
	ids, emails := make(map[string]bool), make(map[string]bool)
	for _, sender := range cfg.Senders {
		if sender.ID == "" || strings.IndexFunc(sender.ID, unicode.IsSpace) >= 0 || ids[sender.ID] {
			return cfg, errors.New("EMAIL_VERIFIED_SENDERS requires unique nonempty IDs without whitespace")
		}
		address, err := mail.ParseAddress(sender.Email)
		email := strings.ToLower(sender.Email)
		if strings.ContainsAny(sender.Email+sender.Name, "\r\n") || err != nil || address.Address != sender.Email || emails[email] {
			return cfg, errors.New("EMAIL_VERIFIED_SENDERS requires unique bare email addresses and safe display names")
		}
		ids[sender.ID], emails[email] = true, true
	}
	return cfg, nil
}
