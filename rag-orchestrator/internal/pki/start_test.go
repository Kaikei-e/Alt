package pki

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStart_DisabledLogs(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeDisabled)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	h, err := Start(context.Background(), log, "rag-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	if h != nil {
		t.Fatal("disabled start must not return a handle")
	}
	if !strings.Contains(buf.String(), "pki_enrollment_disabled") {
		t.Fatalf("log=%s", buf.String())
	}
}

func TestStart_EnabledDoesNotRequireStepBinary(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-backend")
	t.Setenv("STEP_BINARY", filepath.Join(t.TempDir(), "no-such-step"))
	t.Setenv("STEP_CA_URL", "https://127.0.0.1:1")
	t.Setenv("STEP_CA_ROOT_FILE", filepath.Join(t.TempDir(), "missing-root.pem"))
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", filepath.Join(t.TempDir(), "pki-agent-alt-backend-jwk"))
	_, err := Start(context.Background(), slog.Default(), "rag-orchestrator")
	if err == nil {
		t.Fatal("expected native issuer to fail without CA materials")
	}
	if strings.Contains(err.Error(), "step CLI") {
		t.Fatalf("enabled startup must not depend on step executable: %v", err)
	}
}

func TestStartWith_EnabledEnrollsAndStops(t *testing.T) {
	dir := t.TempDir()
	nb := time.Now().Add(-time.Minute)
	cfg := &Config{
		Mode:            ModeEnabled,
		Subject:         "rag-orchestrator",
		SANs:            []string{"rag-orchestrator"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-rag-orchestrator",
		PasswordFile:    "/run/secrets/pki-agent-rag-orchestrator-jwk",
		RenewAtFraction: 0.66,
		TickInterval:    time.Hour,
		RetryAttempts:   1,
		RetryBackoff:    time.Millisecond,
	}
	iss := &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := withT(context.Background(), t)
	h, err := StartWith(ctx, log, cfg, iss)
	if err != nil {
		t.Fatal(err)
	}
	if h == nil {
		t.Fatal("expected handle")
	}
	defer h.Stop()
	if _, err := os.Stat(cfg.CertPath); err != nil {
		t.Fatal(err)
	}
	logs := buf.String()
	if !strings.Contains(logs, "pki_enrollment_enabled") {
		t.Fatalf("log=%s", logs)
	}
	if !strings.Contains(logs, `"password_file_configured":true`) {
		t.Fatalf("expected password_file_configured=true, log=%s", logs)
	}
	assertNoPasswordFileInLogs(t, logs, cfg.PasswordFile, "/run/secrets/")
	h.Stop()
}

func TestStart_EnabledSharedSecretRejected(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "rag-orchestrator")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	_, err := Start(context.Background(), log, "rag-orchestrator")
	if !errors.Is(err, ErrSharedRootSecret) {
		t.Fatalf("got %v", err)
	}
	assertNoPasswordFileInError(t, err, "/run/secrets/", "/run/secrets/step_ca_root_password")
	assertNoPasswordFileInLogs(t, buf.String(), "/run/secrets/", "/run/secrets/step_ca_root_password")
}
