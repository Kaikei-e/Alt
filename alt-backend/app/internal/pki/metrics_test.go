package pki

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestPromObserver_ExportsLifecycleMetrics(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	obs := NewPromObserver("alt-backend", reg)

	obs.OnClassified(StateFresh, 8*time.Hour)
	body := gather(t, reg)
	if !strings.Contains(body, `pki_enrollment_healthy{subject="alt-backend"} 1`) {
		t.Fatalf("healthy missing:\n%s", body)
	}
	if !strings.Contains(body, `pki_enrollment_cert_remaining_seconds{subject="alt-backend"} 28800`) {
		t.Fatalf("remaining missing:\n%s", body)
	}

	obs.OnRenewed(true)
	obs.OnRenewed(false)
	obs.OnReissued("expired")
	obs.OnClassified(StateExpired, -time.Minute)

	body = gather(t, reg)
	for _, needle := range []string{
		`pki_enrollment_healthy{subject="alt-backend"} 0`,
		`pki_enrollment_renewal_total{result="success",subject="alt-backend"} 1`,
		`pki_enrollment_renewal_total{result="failure",subject="alt-backend"} 1`,
		`pki_enrollment_reissue_total{reason="expired",subject="alt-backend"} 1`,
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("ops /metrics missing %q\n%s", needle, body)
		}
	}
}

func TestPromObserver_NilRegistererUsesDefault(t *testing.T) {
	obs := NewPromObserver("alt-harvester-metrics-nil", nil)
	if obs == nil {
		t.Fatal("expected observer")
	}
	obs.OnClassified(StateFresh, time.Hour)
}

func TestStartWithObserver_WiresPromObserverWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	nb := time.Now().Add(-time.Minute)
	cfg := &Config{
		Mode:            ModeEnabled,
		Subject:         "alt-data-hub-metrics",
		SANs:            []string{"alt-data-hub-metrics"},
		CertPath:        dir + "/svc-cert.pem",
		KeyPath:         dir + "/svc-key.pem",
		Provisioner:     "pki-agent-alt-data-hub-metrics",
		PasswordFile:    "/run/secrets/pki-agent-alt-data-hub-metrics-jwk",
		RenewAtFraction: 0.66,
		TickInterval:    time.Hour,
		RetryAttempts:   1,
		RetryBackoff:    time.Millisecond,
	}
	reg := prometheus.NewPedanticRegistry()
	iss := &fakeIssuer{notBefore: nb, lifetime: 24 * time.Hour}
	ctx := withT(t.Context(), t)
	h, err := StartWithObserver(ctx, nil, cfg, iss, NewPromObserver(cfg.Subject, reg))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	body := gather(t, reg)
	if !strings.Contains(body, `pki_enrollment_healthy{subject="alt-data-hub-metrics"} 1`) {
		t.Fatalf("healthy after enroll missing:\n%s", body)
	}
}

func gather(t *testing.T, reg prometheus.Gatherer) string {
	t.Helper()
	rec := httptest.NewRecorder()
	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(
		rec, httptest.NewRequest(http.MethodGet, "/metrics", nil),
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status %d", rec.Code)
	}
	return rec.Body.String()
}
