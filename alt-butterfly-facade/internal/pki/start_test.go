package pki

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestStart_DisabledLogs(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeDisabled)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	h, err := Start(context.Background(), log, "alt-butterfly-facade")
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
	t.Setenv("CERT_SUBJECT", "alt-butterfly-facade")
	t.Setenv("STEP_BINARY", filepath.Join(t.TempDir(), "no-such-step"))
	t.Setenv("STEP_CA_URL", "https://127.0.0.1:1")
	t.Setenv("STEP_CA_ROOT_FILE", filepath.Join(t.TempDir(), "missing-root.pem"))
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", filepath.Join(t.TempDir(), "missing-jwk"))
	_, err := Start(context.Background(), slog.Default(), "alt-butterfly-facade")
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
		Subject:         "alt-butterfly-facade",
		SANs:            []string{"alt-butterfly-facade"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-alt-butterfly-facade",
		PasswordFile:    "/run/secrets/pki-agent-alt-butterfly-facade-jwk",
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

func TestStartEnabled_PrivateRegistryNotDefault(t *testing.T) {
	dir := t.TempDir()
	nb := time.Now().Add(-time.Minute)
	cfg := &Config{
		Mode:            ModeEnabled,
		Subject:         "alt-butterfly-facade-private-reg",
		SANs:            []string{"alt-butterfly-facade-private-reg"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-alt-butterfly-facade-private-reg",
		PasswordFile:    "/run/secrets/pki-agent-alt-butterfly-facade-private-reg-jwk",
		RenewAtFraction: 0.66,
		TickInterval:    time.Hour,
		RetryAttempts:   1,
		RetryBackoff:    time.Millisecond,
	}
	iss := &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}
	ctx := withT(context.Background(), t)
	h, err := startEnabled(ctx, slog.Default(), cfg, iss)
	if err != nil {
		t.Fatal(err)
	}
	if h == nil {
		t.Fatal("expected handle")
	}
	defer h.Stop()
	mh := h.MetricsHandler()
	if mh == nil {
		t.Fatal("enabled start must expose a private-registry metrics handler")
	}
	rec := httptest.NewRecorder()
	mh.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status %d", rec.Code)
	}
	body := rec.Body.String()
	needle := `pki_enrollment_healthy{subject="alt-butterfly-facade-private-reg"} 1`
	if !strings.Contains(body, needle) {
		t.Fatalf("ops scrape missing %q\n%s", needle, body)
	}
	def := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(def, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if strings.Contains(def.Body.String(), `subject="alt-butterfly-facade-private-reg"`) {
		t.Fatal("PKI series leaked onto prometheus.DefaultGatherer")
	}
}

func TestStart_EnabledSharedSecretRejected(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "alt-butterfly-facade")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	_, err := Start(context.Background(), log, "alt-butterfly-facade")
	if !errors.Is(err, ErrSharedRootSecret) {
		t.Fatalf("got %v", err)
	}
	assertNoPasswordFileInError(t, err, "/run/secrets/", "/run/secrets/step_ca_root_password")
	assertNoPasswordFileInLogs(t, buf.String(), "/run/secrets/", "/run/secrets/step_ca_root_password")
}
