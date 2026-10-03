package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRagOrchestratorHealthcheck_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("expected path /healthz, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	_, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	exitCode := runHealthcheck(port, ts.Client())
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}

func TestRagOrchestratorHealthcheck_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unavailable"}`))
	}))
	defer ts.Close()

	_, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	exitCode := runHealthcheck(port, ts.Client())
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for 503, got %d", exitCode)
	}
}

func TestRagOrchestratorHealthcheck_ConnectionRefused(t *testing.T) {
	// Pick an unused port by briefly binding and releasing
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}
	_ = listener.Close()

	exitCode := runHealthcheck(port, nil)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for connection refused, got %d", exitCode)
	}
}

func TestRagOrchestratorHealthcheck_ArgParsing(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		env          string
		wantIsHealth bool
		wantPort     string
	}{
		{
			name:         "plain healthcheck default",
			args:         []string{"/rag-orchestrator", "healthcheck"},
			wantIsHealth: true,
			wantPort:     "9012",
		},
		{
			name:         "healthcheck with -port flag",
			args:         []string{"/rag-orchestrator", "healthcheck", "-port", "9015"},
			wantIsHealth: true,
			wantPort:     "9015",
		},
		{
			name:         "healthcheck with --port= flag",
			args:         []string{"/rag-orchestrator", "healthcheck", "--port=9016"},
			wantIsHealth: true,
			wantPort:     "9016",
		},
		{
			name:         "healthcheck with positional port",
			args:         []string{"/rag-orchestrator", "healthcheck", "9017"},
			wantIsHealth: true,
			wantPort:     "9017",
		},
		{
			name:         "healthcheck with env port",
			args:         []string{"/rag-orchestrator", "healthcheck"},
			env:          "9018",
			wantIsHealth: true,
			wantPort:     "9018",
		},
		{
			name:         "docker entrypoint wrapping",
			args:         []string{"/rag-orchestrator", "/rag-orchestrator", "healthcheck", "9019"},
			wantIsHealth: true,
			wantPort:     "9019",
		},
		{
			name:         "non-healthcheck command",
			args:         []string{"/rag-orchestrator"},
			wantIsHealth: false,
			wantPort:     "9012",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("RAG_HEALTH_PORT", tc.env)
			}
			isH := isHealthcheckCommand(tc.args)
			if isH != tc.wantIsHealth {
				t.Errorf("isHealthcheckCommand(%v) = %v, want %v", tc.args, isH, tc.wantIsHealth)
			}
			port := extractHealthcheckPort(tc.args)
			if port != tc.wantPort {
				t.Errorf("extractHealthcheckPort(%v) = %v, want %v", tc.args, port, tc.wantPort)
			}
		})
	}
}

func TestRagOrchestratorHealthcheck_NoSecretsRequired(t *testing.T) {
	// Verify that runHealthcheck can be called without any DB or PKI setup
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	_, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	// Unset relevant secrets and verify probe still executes cleanly
	t.Setenv("DB_PASSWORD_FILE", "/nonexistent/password")
	t.Setenv("RAG_API_TOKEN_FILE", "/nonexistent/token")
	t.Setenv("SOVEREIGN_EVENT_TOKEN_FILE", "/nonexistent/sovereign")

	exitCode := runHealthcheck(port, ts.Client())
	if exitCode != 0 {
		t.Fatalf("runHealthcheck should succeed even when secrets files do not exist: got %d", exitCode)
	}
}

func TestRagOrchestratorHealthcheck_PortValidation(t *testing.T) {
	var networkHits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&networkHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	invalidPorts := []string{
		"0",
		"65536",
		"-1",
		"80/foo",
		"user:pass@8080",
		" 8080 ",
		"abc",
		"",
		"-9012",
		"9012 ",
		" 9012",
	}

	for _, p := range invalidPorts {
		t.Run("invalid_"+p, func(t *testing.T) {
			hitsBefore := atomic.LoadInt32(&networkHits)
			code := runHealthcheck(p, ts.Client())
			if code == 0 {
				t.Errorf("expected non-zero exit code for invalid port %q", p)
			}
			hitsAfter := atomic.LoadInt32(&networkHits)
			if hitsAfter != hitsBefore {
				t.Errorf("expected 0 network calls for invalid port %q, got %d hits", p, hitsAfter-hitsBefore)
			}
		})
	}
}

func TestRagOrchestratorHealthcheck_RedirectBlocked(t *testing.T) {
	var destHits int32
	destTs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&destHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer destTs.Close()

	var originHits int32
	originTs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&originHits, 1)
		http.Redirect(w, r, destTs.URL, http.StatusFound) // 302 Found
	}))
	defer originTs.Close()

	_, port, err := net.SplitHostPort(originTs.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	code := runHealthcheck(port, originTs.Client())
	if code == 0 {
		t.Fatalf("expected non-zero exit code for 302 redirect, got 0")
	}

	if atomic.LoadInt32(&originHits) != 1 {
		t.Errorf("expected 1 origin hit, got %d", atomic.LoadInt32(&originHits))
	}
	if atomic.LoadInt32(&destHits) != 0 {
		t.Errorf("expected 0 dest hits (redirect must not be followed), got %d", atomic.LoadInt32(&destHits))
	}
}

func TestRagOrchestratorHealthcheck_OversizeBodyFailClosed(t *testing.T) {
	body4096 := bytes.Repeat([]byte("a"), 4096)
	ts4096 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body4096)
	}))
	defer ts4096.Close()

	_, port4096, _ := net.SplitHostPort(ts4096.Listener.Addr().String())
	if code := runHealthcheck(port4096, ts4096.Client()); code != 0 {
		t.Errorf("expected 0 for 4096 byte body, got %d", code)
	}

	body4097 := bytes.Repeat([]byte("a"), 4097)
	ts4097 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body4097)
	}))
	defer ts4097.Close()

	_, port4097, _ := net.SplitHostPort(ts4097.Listener.Addr().String())
	if code := runHealthcheck(port4097, ts4097.Client()); code == 0 {
		t.Errorf("expected non-zero (fail-closed) for 4097 byte body, got 0")
	}
}

func TestRagOrchestratorHealthcheck_ClientCloneNoMutation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	_, port, _ := net.SplitHostPort(ts.Listener.Addr().String())

	customCheckRedirectCalled := false
	originalClient := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			customCheckRedirectCalled = true
			return nil
		},
	}

	code := runHealthcheck(port, originalClient)
	if code != 0 {
		t.Fatalf("expected 0 exit code, got %d", code)
	}

	if originalClient.Timeout != 10*time.Second {
		t.Errorf("original client Timeout was mutated: got %v, want 10s", originalClient.Timeout)
	}
	if originalClient.CheckRedirect == nil {
		t.Errorf("original client CheckRedirect was nil")
	}
	if customCheckRedirectCalled {
		t.Errorf("original client CheckRedirect was called unexpectedly")
	}
}
