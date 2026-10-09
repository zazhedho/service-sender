package database

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestConnDbDoesNotLogDsn(t *testing.T) {
	const helper = "SERVICE_SENDER_DSN_LOG_HELPER"
	if os.Getenv(helper) == "1" {
		_, _, _ = ConnDb()
		return
	}

	secret := "dsn-password-test-sentinel"
	dsn := "host=127.0.0.1 port=1 user=test password=" + secret + " dbname=test sslmode=disable connect_timeout=1"
	cmd := exec.Command(os.Args[0], "-test.run=^TestConnDbDoesNotLogDsn$")
	cmd.Env = replaceEnv(os.Environ(), map[string]string{
		helper: "1", "DATABASE_URL": dsn, "LOG_FORMAT": "text", "LOG_LEVEL": "5",
	})
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("database log helper failed: %v\n%s", err, output)
	}
	if strings.Contains(string(output), secret) || strings.Contains(string(output), dsn) {
		t.Fatalf("database logs exposed the DSN: %s", output)
	}
}

func replaceEnv(env []string, replacements map[string]string) []string {
	result := make([]string, 0, len(env)+len(replacements))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[key]; !replaced {
			result = append(result, entry)
		}
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}
