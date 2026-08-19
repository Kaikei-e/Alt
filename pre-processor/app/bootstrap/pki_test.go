package bootstrap

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"pre-processor/internal/pki"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRunSourceWiresEnrollmentBeforeTLSConsumers(t *testing.T) {
	src := readBootstrapSource(t, "lifecycle.go")
	enroll := strings.Index(src, "startEnrollment(")
	build := strings.Index(src, "BuildDependencies(")
	httpStart := strings.Index(src, "StartHTTPServer(")
	connectStart := strings.Index(src, "StartConnectServer(")
	if enroll < 0 {
		t.Fatal("lifecycle.go must call startEnrollment")
	}
	if build < 0 || enroll > build {
		t.Fatal("enrollment must start before BuildDependencies (mTLS client)")
	}
	if httpStart < 0 || connectStart < 0 || enroll > httpStart || enroll > connectStart {
		t.Fatal("enrollment must start before HTTP/Connect listeners")
	}
	if !strings.Contains(src, "pki.ListenOps(") {
		t.Fatal("lifecycle.go must start the dedicated PKI ops listener")
	}
	if !strings.Contains(src, "pki.ShutdownOps(") {
		t.Fatal("lifecycle.go must shut down the dedicated PKI ops listener")
	}
	if strings.Contains(src, "attachPKIMetrics") {
		t.Fatal("PKI metrics must not attach to :9201; scrape is parent :9110")
	}
}

func TestStartEnrollment_Disabled(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", pki.ModeDisabled)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	enroll, err := startEnrollment(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}
	defer enroll.stop()
	if enroll.gatherer != nil {
		t.Fatal("disabled enrollment must not export a gatherer")
	}
	if !strings.Contains(buf.String(), "pki_enrollment_disabled") {
		t.Fatalf("log=%s", buf.String())
	}
}

func TestStartEnrollment_EnabledRejectsSharedRoot(t *testing.T) {
	t.Setenv("PKI_ENROLLMENT", pki.ModeEnabled)
	t.Setenv("CERT_SUBJECT", "pre-processor")
	t.Setenv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password")
	if _, err := startEnrollment(context.Background(), slog.Default()); err == nil {
		t.Fatal("expected shared root secret to fail")
	}
}

func TestEnrollmentMetricsHandler_PrivateRegistry(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	obs := pki.NewPromObserver("pre-processor-metrics", reg)
	obs.OnClassified(pki.StateFresh, 8*time.Hour)
	enroll := &pkiEnrollment{gatherer: reg}
	h := enroll.metricsHandler()
	if h == nil {
		t.Fatal("enabled enrollment must export a metrics handler for :9110")
	}
}

func TestEnrollmentMetricsHandler_NilWhenDisabled(t *testing.T) {
	if (&pkiEnrollment{}).metricsHandler() != nil {
		t.Fatal("disabled enrollment must not export PKI metrics")
	}
}

func readBootstrapSource(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), name)
	body, err := os.ReadFile(path) // #nosec G304 -- test fixture path from runtime.Caller
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
