package pki

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestLoadOpsListenAddr_Default(t *testing.T) {
	unsetenv(t, "OPS_LISTEN")
	got, err := LoadOpsListenAddr()
	if err != nil {
		t.Fatal(err)
	}
	if got != defaultOpsListenAddr {
		t.Fatalf("got %q want %q", got, defaultOpsListenAddr)
	}
}

func TestLoadOpsListenAddr_Override(t *testing.T) {
	t.Setenv("OPS_LISTEN", ":9110")
	got, err := LoadOpsListenAddr()
	if err != nil {
		t.Fatal(err)
	}
	if got != ":9110" {
		t.Fatalf("got %q", got)
	}
}

func TestNewOpsHandler_ServesPrivateRegistryOnly(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	obs := NewPromObserver("rag-orchestrator", reg)
	obs.OnClassified(StateFresh, time.Hour)

	srv := httptest.NewServer(NewOpsHandler(reg))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `pki_enrollment_healthy{subject="rag-orchestrator"} 1`) {
		t.Fatalf("ops metrics missing PKI series:\n%s", body)
	}

	defaultBody, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range defaultBody {
		if strings.Contains(mf.GetName(), "pki_enrollment") && strings.Contains(mf.String(), "rag-orchestrator") {
			t.Fatal("PKI collector leaked onto DefaultRegisterer")
		}
	}
}
