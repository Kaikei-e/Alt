package pki

import (
	"bytes"
	"context"
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
	h, err := Start(context.Background(), log, "search-indexer")
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
	t.Setenv("CERT_SUBJECT", "search-indexer")
	t.Setenv("STEP_BINARY", filepath.Join(t.TempDir(), "no-such-step"))
	t.Setenv("STEP_CA_URL", "https://127.0.0.1:1")
	t.Setenv("STEP_CA_ROOT_FILE", filepath.Join(t.TempDir(), "missing-root.pem"))
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", filepath.Join(t.TempDir(), "missing-jwk"))
	_, err := Start(context.Background(), slog.Default(), "search-indexer")
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
		Subject:         "search-indexer",
		SANs:            []string{"search-indexer"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-search-indexer",
		PasswordFile:    "/run/secrets/pki-agent-search-indexer-jwk",
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
	if !strings.Contains(buf.String(), "pki_enrollment_enabled") {
		t.Fatalf("log=%s", buf.String())
	}
	h.Stop()
}

func TestStartEnabled_PrivateRegistryNotDefault(t *testing.T) {
	dir := t.TempDir()
	nb := time.Now().Add(-time.Minute)
	cfg := &Config{
		Mode:            ModeEnabled,
		Subject:         "search-indexer-private-reg",
		SANs:            []string{"search-indexer-private-reg"},
		CertPath:        filepath.Join(dir, "svc-cert.pem"),
		KeyPath:         filepath.Join(dir, "svc-key.pem"),
		Provisioner:     "pki-agent-search-indexer-private-reg",
		PasswordFile:    "/run/secrets/pki-agent-search-indexer-private-reg-jwk",
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
	needle := `pki_enrollment_healthy{subject="search-indexer-private-reg"} 1`
	if !strings.Contains(body, needle) {
		t.Fatalf("ops scrape missing %q\n%s", needle, body)
	}
	def := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(def, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if strings.Contains(def.Body.String(), `subject="search-indexer-private-reg"`) {
		t.Fatal("PKI series leaked onto prometheus.DefaultGatherer")
	}
}

func TestStart_EnabledSharedSecretRejected(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", ModeEnabled)
	t.Setenv("CERT_SUBJECT", "search-indexer")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
	if _, err := Start(context.Background(), slog.Default(), "search-indexer"); err == nil {
		t.Fatal("expected shared root secret to fail")
	}
}
