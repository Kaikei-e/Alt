package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"pki-agent/internal/domain"
)

func TestLogTickFailure_DoesNotLogPasswordFile(t *testing.T) {
	secretPath := "/run/secrets/pki-agent-tag-generator-jwk"
	password := "super-secret-jwk-password"
	leak := fmt.Errorf("open %s: %s", secretPath, password)

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	logTickFailure(log, "initial tick failed", leak, "missing", 0)
	logTickFailure(log, "tick failed", leak, "expired", 3)
	logs := buf.String()
	if !strings.Contains(logs, `"error_type"`) {
		t.Fatalf("expected error_type, got %s", logs)
	}
	if !strings.Contains(logs, "initial tick failed") || !strings.Contains(logs, "tick failed") {
		t.Fatalf("expected tick failure messages, got %s", logs)
	}
	for _, leakStr := range []string{secretPath, "/run/secrets/", password, "-jwk"} {
		if strings.Contains(logs, leakStr) {
			t.Fatalf("secret material %q leaked in logs: %s", leakStr, logs)
		}
	}
}

func TestLogSafeError_DoesNotEchoPath(t *testing.T) {
	err := fmt.Errorf("issue cert: %w: %s", domain.ErrTokenSign, "/run/secrets/pki-agent-tag-generator-jwk")
	got := domain.LogSafeError(err)
	if got != "token_sign" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "/run/secrets/") || strings.Contains(got, "jwk") {
		t.Fatalf("error_type leaked path: %q", got)
	}
}

func TestLogConfigFailure_DoesNotLogPasswordFile(t *testing.T) {
	secretPath := "/run/secrets/pki-agent-tag-generator-jwk"
	password := "super-secret-jwk-password"
	err := fmt.Errorf("STEP_CA_PROVISIONER_PASSWORD_FILE: open %s: %s", secretPath, password)

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	logConfigFailure(log, err)
	logs := buf.String()
	if !strings.Contains(logs, `"msg":"config"`) {
		t.Fatalf("expected config failure log, got %s", logs)
	}
	if !strings.Contains(logs, `"error_type"`) {
		t.Fatalf("expected error_type, got %s", logs)
	}
	for _, leakStr := range []string{secretPath, "/run/secrets/", password, "-jwk", `"err"`} {
		if strings.Contains(logs, leakStr) {
			t.Fatalf("secret material %q leaked in config logs: %s", leakStr, logs)
		}
	}
}
