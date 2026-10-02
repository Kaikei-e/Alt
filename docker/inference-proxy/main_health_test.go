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

func TestInferenceProxyHealthcheck_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
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

func TestInferenceProxyHealthcheck_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer ts.Close()

	_, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	exitCode := runHealthcheck(port, ts.Client())
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for 500, got %d", exitCode)
	}
}

func TestInferenceProxyHealthcheck_ConnectionRefused(t *testing.T) {
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

func TestInferenceProxyHealthcheck_ArgParsing(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		envPort      string
		envListen    string
		wantIsHealth bool
		wantPort     string
	}{
		{
			name:         "plain healthcheck default",
			args:         []string{"/inference-proxy", "healthcheck"},
			wantIsHealth: true,
			wantPort:     "11436",
		},
		{
			name:         "healthcheck with -port flag",
			args:         []string{"/inference-proxy", "healthcheck", "-port", "11438"},
			wantIsHealth: true,
			wantPort:     "11438",
		},
		{
			name:         "healthcheck with --port= flag",
			args:         []string{"/inference-proxy", "healthcheck", "--port=11439"},
			wantIsHealth: true,
			wantPort:     "11439",
		},
		{
			name:         "healthcheck with positional port",
			args:         []string{"/inference-proxy", "healthcheck", "11440"},
			wantIsHealth: true,
			wantPort:     "11440",
		},
		{
			name:         "healthcheck with PORT env",
			args:         []string{"/inference-proxy", "healthcheck"},
			envPort:      "11441",
			wantIsHealth: true,
			wantPort:     "11441",
		},
		{
			name:         "healthcheck with LISTEN_ADDR env",
			args:         []string{"/inference-proxy", "healthcheck"},
			envListen:    ":11442",
			wantIsHealth: true,
			wantPort:     "11442",
		},
		{
			name:         "docker entrypoint wrapping",
			args:         []string{"/inference-proxy", "/inference-proxy", "healthcheck", "-port", "11443"},
			wantIsHealth: true,
			wantPort:     "11443",
		},
		{
			name:         "non-healthcheck command",
			args:         []string{"/inference-proxy", "--token-file", "/run/secrets/token"},
			wantIsHealth: false,
			wantPort:     "11436",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envPort != "" {
				t.Setenv("PORT", tc.envPort)
			}
			if tc.envListen != "" {
				t.Setenv("LISTEN_ADDR", tc.envListen)
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

func TestInferenceProxyHealthcheck_NoSecretsRequired(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	_, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	// Token file does not exist, probe must still succeed
	t.Setenv("INFERENCE_TOKEN_FILE", "/nonexistent/token")

	exitCode := runHealthcheck(port, ts.Client())
	if exitCode != 0 {
		t.Fatalf("runHealthcheck should succeed even when token file is absent: got %d", exitCode)
	}
}

func TestInferenceProxyHealthcheck_PortValidation(t *testing.T) {
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
		"-11436",
		"11436 ",
		" 11436",
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

func TestInferenceProxyHealthcheck_RedirectBlocked(t *testing.T) {
	var destHits int32
	destTs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&destHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
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

func TestInferenceProxyHealthcheck_OversizeBodyFailClosed(t *testing.T) {
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

func TestInferenceProxyHealthcheck_ClientCloneNoMutation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
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
