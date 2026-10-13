package config

import (
	"strings"
	"testing"
)

const validMCPSenders = `[{"id":"support","email":"support@example.com","name":"Tim Bantuan"}]`

func TestLoadMCPConfig(t *testing.T) {
	tests := []struct {
		name, enabled, addr, key, senders string
		wantError                         bool
	}{
		{name: "disabled"},
		{name: "disabled ignores unused credentials", enabled: "false", senders: "not-json"},
		{name: "enabled", enabled: "true", key: "test-key", senders: validMCPSenders},
		{name: "custom address", enabled: "true", addr: "localhost:8091", key: "test-key", senders: validMCPSenders},
		{name: "invalid boolean", enabled: "yes", wantError: true},
		{name: "missing key", enabled: "true", senders: validMCPSenders, wantError: true},
		{name: "blank key", enabled: "true", key: "  ", senders: validMCPSenders, wantError: true},
		{name: "token whitespace", enabled: "true", key: "two words", senders: validMCPSenders, wantError: true},
		{name: "header key", enabled: "true", key: "key\r\nInjected: yes", senders: validMCPSenders, wantError: true},
		{name: "missing list", enabled: "true", key: "test-key", wantError: true},
		{name: "empty list", enabled: "true", key: "test-key", senders: "[]", wantError: true},
		{name: "null list", enabled: "true", key: "test-key", senders: "null", wantError: true},
		{name: "invalid JSON", enabled: "true", key: "test-key", senders: "[", wantError: true},
		{name: "trailing JSON", enabled: "true", key: "test-key", senders: validMCPSenders + "[]", wantError: true},
		{name: "unknown field", enabled: "true", key: "test-key", senders: `[{"id":"support","email":"support@example.com","typo":"name"}]`, wantError: true},
		{name: "empty ID", enabled: "true", key: "test-key", senders: `[{"email":"support@example.com"}]`, wantError: true},
		{name: "ID whitespace", enabled: "true", key: "test-key", senders: `[{"id":"a b","email":"support@example.com"}]`, wantError: true},
		{name: "duplicate ID", enabled: "true", key: "test-key", senders: `[{"id":"a","email":"a@example.com"},{"id":"a","email":"b@example.com"}]`, wantError: true},
		{name: "duplicate email", enabled: "true", key: "test-key", senders: `[{"id":"a","email":"a@example.com"},{"id":"b","email":"A@example.com"}]`, wantError: true},
		{name: "invalid email", enabled: "true", key: "test-key", senders: `[{"id":"a","email":"invalid"}]`, wantError: true},
		{name: "display address", enabled: "true", key: "test-key", senders: `[{"id":"a","email":"Name <a@example.com>"}]`, wantError: true},
		{name: "two mailboxes", enabled: "true", key: "test-key", senders: `[{"id":"a","email":"a@example.com,b@example.com"}]`, wantError: true},
		{name: "header email", enabled: "true", key: "test-key", senders: `[{"id":"a","email":"a@example.com\r\nBcc: x@example.com"}]`, wantError: true},
		{name: "header name", enabled: "true", key: "test-key", senders: `[{"id":"a","email":"a@example.com","name":"A\nBcc: x@example.com"}]`, wantError: true},
		{name: "invalid address", enabled: "true", addr: "localhost", key: "test-key", senders: validMCPSenders, wantError: true},
		{name: "invalid port", enabled: "true", addr: "localhost:70000", key: "test-key", senders: validMCPSenders, wantError: true},
		{name: "zero port", enabled: "true", addr: "localhost:0", key: "test-key", senders: validMCPSenders, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMCPConfig(t, tt)
		})
	}
}

func assertMCPConfig(t *testing.T, tt struct {
	name, enabled, addr, key, senders string
	wantError                         bool
}) {
	t.Helper()
	t.Setenv("MCP_ENABLED", tt.enabled)
	t.Setenv("MCP_ADDR", tt.addr)
	t.Setenv("MCP_SERVER_KEY", tt.key)
	t.Setenv("EMAIL_VERIFIED_SENDERS", tt.senders)
	cfg, err := LoadMCPConfig()
	if (err != nil) != tt.wantError {
		t.Fatalf("error = %v, wantError %v", err, tt.wantError)
	}
	if tt.wantError {
		if tt.key != "" && strings.Contains(err.Error(), tt.key) {
			t.Fatal("configuration error exposes key")
		}
		return
	}
	if cfg.Enabled != (tt.enabled == "true") {
		t.Fatalf("enabled = %v", cfg.Enabled)
	}
	wantAddr := "127.0.0.1:8081"
	if tt.addr != "" {
		wantAddr = tt.addr
	}
	if cfg.Addr != wantAddr {
		t.Fatalf("address = %q", cfg.Addr)
	}
	if cfg.Enabled && (len(cfg.Senders) != 1 || cfg.Senders[0].Email != "support@example.com") {
		t.Fatalf("senders = %+v", cfg.Senders)
	}
}

func TestLoadMCPConfigPreservesSenderOrder(t *testing.T) {
	t.Setenv("MCP_ENABLED", "true")
	t.Setenv("MCP_ADDR", "")
	t.Setenv("MCP_SERVER_KEY", "test-key")
	t.Setenv("EMAIL_VERIFIED_SENDERS", `[{"id":"b","email":"b@example.com","name":"Bantuan, 日本語"},{"id":"a","email":"a@example.com","name":"A"}]`)
	cfg, err := LoadMCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Senders) != 2 || cfg.Senders[0].ID != "b" || cfg.Senders[0].Name != "Bantuan, 日本語" || cfg.Senders[1].ID != "a" {
		t.Fatalf("senders = %+v", cfg.Senders)
	}
}
