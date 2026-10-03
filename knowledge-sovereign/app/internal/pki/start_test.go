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
	h, err := Start(context.Background(), log, "knowledge-sovereign")
	if err != nil {
		t.Fatal(err)
	}
	if h != nil {
		t.Fatal("disabled start must not return a handle")
	}
	out := buf.String()
	if !strings.Contains(out, "pki_enrollment_disabled") || !strings.Contains(out, `"level":"WARN"`) {
		t.Fatalf("an explicit opt-out must log a warning: %s", out)
	}
	if !strings.Contains(out, "PKI_ENROLLMENT=disabled") {
		t.Fatalf("the reason must name the explicit setting: %s", out)
	}
	if strings.Contains(out, "sidecar") {
		t.Fatalf("no pki-agent sidecar owns the cert files any more: %s", out)
	}
}

func TestStart_UnsetEnrollmentFails(t *testing.T) {
	unsetEnrollmentEnv(t)
	h, err := Start(context.Background(), slog.New(slog.DiscardHandler), "knowledge-sovereign")
	if err == nil {
		h.Stop()
		t.Fatal("unset PKI_ENROLLMENT must fail startup")
	}
}

func TestStart_EnabledDoesNotRequireStepBinary(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "knowledge-sovereign")
	t.Setenv("STEP_BINARY", filepath.Join(t.TempDir(), "no-such-step"))
	t.Setenv("STEP_CA_URL", "https://127.0.0.1:1")
	t.Setenv("STEP_CA_ROOT_FILE", filepath.Join(t.TempDir(), "missing-root.pem"))
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", filepath.Join(t.TempDir(), "missing-jwk"))
	_, err := Start(context.Background(), slog.Default(), "knowledge-sovereign")
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
		Subject:         "knowledge-sovereign",
		SANs:            []string{"knowledge-sovereign"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-knowledge-sovereign",
		PasswordFile:    "/run/secrets/pki-agent-knowledge-sovereign-jwk",
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
		Subject:         "knowledge-sovereign-private-reg",
		SANs:            []string{"knowledge-sovereign-private-reg"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-knowledge-sovereign-private-reg",
		PasswordFile:    "/run/secrets/pki-agent-knowledge-sovereign-private-reg-jwk",
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
	needle := `pki_enrollment_healthy{subject="knowledge-sovereign-private-reg"} 1`
	if !strings.Contains(body, needle) {
		t.Fatalf("ops scrape missing %q\n%s", needle, body)
	}
	def := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(def, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if strings.Contains(def.Body.String(), `subject="knowledge-sovereign-private-reg"`) {
		t.Fatal("PKI series leaked onto prometheus.DefaultGatherer")
	}
}

func TestStart_EnabledSharedSecretRejected(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "knowledge-sovereign")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	_, err := Start(context.Background(), log, "knowledge-sovereign")
	if !errors.Is(err, ErrSharedRootSecret) {
		t.Fatalf("got %v", err)
	}
	assertNoPasswordFileInError(t, err, "/run/secrets/", "/run/secrets/step_ca_root_password")
	assertNoPasswordFileInLogs(t, buf.String(), "/run/secrets/", "/run/secrets/step_ca_root_password")
}
