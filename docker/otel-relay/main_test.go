package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func writeTokenFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "token*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

// originCapture starts a test HTTP origin server that records every request.
type originCapture struct {
	method      string
	path        string
	body        []byte
	authHeader  string
	contentType string
	status      int // response status to return
}

func newOriginServer(t *testing.T, status int) (*httptest.Server, *originCapture) {
	t.Helper()
	cap := &originCapture{status: status}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.path = r.URL.Path
		cap.authHeader = r.Header.Get("Authorization")
		cap.contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		cap.body = body
		w.WriteHeader(status)
	}))
	t.Cleanup(ts.Close)
	return ts, cap
}

// buildRelay creates a *server with a test origin URL and given token.
func buildRelay(t *testing.T, token, endpoint string) *server {
	t.Helper()
	targetURL, err := parseAndNormalizeTargetURL(endpoint)
	if err != nil {
		t.Fatalf("buildRelay: parseAndNormalizeTargetURL failed: %v", err)
	}
	return &server{
		token:     token,
		targetURL: targetURL,
		client:    newClient(),
	}
}

// ── isValidToken ─────────────────────────────────────────────────────────────

func TestIsValidToken(t *testing.T) {
	cases := []struct {
		tok  string
		want bool
	}{
		{"abc123", true},
		{"tok.en_val~ue+ok/ok-dash", true},
		{"base64padded==", true},
		{"base64padded=", true},
		{"a=", true},
		{"abc==", true},
		{"valid=", true},
		{"A", true},
		{"", false},             // empty
		{"=", false},            // padding-only (1 char)
		{"==", false},           // padding-only (2 chars)
		{"====", false},         // padding-only (4 chars)
		{"has space", false},    // space not allowed
		{"has\nnewline", false}, // newline not allowed
		{"mid=dle=", false},     // '=' not at end
		{"=leading", false},     // leading '='
		{"bad@char", false},     // '@' not allowed
		{"secret\x00", false},   // null byte
	}
	for _, tc := range cases {
		got := isValidToken(tc.tok)
		if got != tc.want {
			t.Errorf("isValidToken(%q) = %v, want %v", tc.tok, got, tc.want)
		}
	}
}

// ── parseAndNormalizeTargetURL ───────────────────────────────────────────────

func TestParseAndNormalizeTargetURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
		wantURL string
	}{
		{"http://rask-log-aggregator:4318", false, "http://rask-log-aggregator:4318/v1/traces"},
		{"http://rask-log-aggregator:4318/", false, "http://rask-log-aggregator:4318/v1/traces"},
		{"http://rask-log-aggregator:4318/v1/traces", false, "http://rask-log-aggregator:4318/v1/traces"},
		{"http://rask-log-aggregator:4318/v1/traces/", false, "http://rask-log-aggregator:4318/v1/traces"},
		{"https://collector.example.com", false, "https://collector.example.com/v1/traces"},
		{"https://collector.example.com/v1/traces", false, "https://collector.example.com/v1/traces"},
		{"http://localhost:4318", false, "http://localhost:4318/v1/traces"},
		{"http://localhost:4318/v1/traces", false, "http://localhost:4318/v1/traces"},
		{"http://[::1]:4318", false, "http://[::1]:4318/v1/traces"},
		{"", true, ""},
		{"http://host:4318/v1/traces/extra", true, ""},
		{"http://host:4318/v1/metrics", true, ""},
		{"http://host:4318/other", true, ""},
		{"ftp://bad-scheme.example", true, ""},
		{"http://user@host:4318", true, ""}, // userinfo not allowed
		{"http://host:4318?q=1", true, ""},  // query not allowed
		{"http://host:4318?", true, ""},     // forcequery not allowed
		{"http://host:4318#frag", true, ""}, // fragment not allowed
		{"/only/path", true, ""},            // no scheme/host
		{"http://", true, ""},               // empty host
		{"http://:4318", true, ""},          // empty hostname
		{"http://collector:4318/%76%31/traces", true, ""}, // encoded path strict reject
	}
	for _, tc := range cases {
		got, err := parseAndNormalizeTargetURL(tc.url)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseAndNormalizeTargetURL(%q): err=%v, wantErr=%v", tc.url, err, tc.wantErr)
		}
		if err == nil && got != tc.wantURL {
			t.Errorf("parseAndNormalizeTargetURL(%q): got=%q, want=%q", tc.url, got, tc.wantURL)
		}
	}
}

// ── loadToken startup ─────────────────────────────────────────────────────────

func TestLoadToken_ValidFile(t *testing.T) {
	path := writeTokenFile(t, "myValidToken123\n")
	tok, err := loadToken(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "myValidToken123" {
		t.Errorf("got token %q, want %q", tok, "myValidToken123")
	}
}

func TestLoadToken_MissingFile(t *testing.T) {
	_, err := loadToken(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadToken_InvalidChars(t *testing.T) {
	path := writeTokenFile(t, "token with spaces")
	_, err := loadToken(path)
	if err == nil {
		t.Fatal("expected error for invalid token charset")
	}
}

func TestLoadToken_EmptyFile(t *testing.T) {
	path := writeTokenFile(t, "")
	_, err := loadToken(path)
	if err == nil {
		t.Fatal("expected error for empty token file")
	}
}

func TestLoadToken_EmptyPath(t *testing.T) {
	_, err := loadToken("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

// ── healthz endpoint ──────────────────────────────────────────────────────────

func TestHealthzGET(t *testing.T) {
	mux := newMux(&server{token: "tok", targetURL: "http://unused", client: newClient()})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /healthz status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "ok") {
		t.Errorf("GET /healthz body %q, want 'ok'", body)
	}
}

func TestHealthzPOSTNotAllowed(t *testing.T) {
	mux := newMux(&server{token: "tok", targetURL: "http://unused", client: newClient()})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /healthz status = %d, want 405", rr.Code)
	}
}

// ── traces endpoint — exact header / path / body forwarding ──────────────────

func TestTracesHandler_ForwardsExactPathAndBody(t *testing.T) {
	origin, cap := newOriginServer(t, http.StatusOK)
	relay := buildRelay(t, "mytoken", origin.URL)
	mux := newMux(relay)

	payload := []byte("protobuf-binary-payload")
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-protobuf")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if cap.path != "/v1/traces" {
		t.Errorf("upstream path = %q, want /v1/traces", cap.path)
	}
	if !bytes.Equal(cap.body, payload) {
		t.Errorf("upstream body = %q, want %q", cap.body, payload)
	}
	if cap.contentType != "application/x-protobuf" {
		t.Errorf("upstream Content-Type = %q, want application/x-protobuf", cap.contentType)
	}
}

func TestTracesHandler_InjectsRelayBearerToken(t *testing.T) {
	origin, cap := newOriginServer(t, http.StatusOK)
	relay := buildRelay(t, "relay-secret-token", origin.URL)
	mux := newMux(relay)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("body")))
	// Forge an incoming Authorization — must NOT be forwarded.
	req.Header.Set("Authorization", "Bearer attacker-forged-token")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if cap.authHeader != "Bearer relay-secret-token" {
		t.Errorf("upstream Authorization = %q, want 'Bearer relay-secret-token'", cap.authHeader)
	}
}

func TestTracesHandler_ForgedAuthNotPassedThrough(t *testing.T) {
	origin, cap := newOriginServer(t, http.StatusOK)
	relay := buildRelay(t, "good-tok", origin.URL)
	mux := newMux(relay)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("x")))
	req.Header.Set("Authorization", "Bearer evil")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if strings.Contains(cap.authHeader, "evil") {
		t.Errorf("forged Authorization leaked to upstream: %q", cap.authHeader)
	}
}

// ── oversize body: upstream must receive ZERO hits ────────────────────────────

func TestTracesHandler_OversizeBody_Returns413_NoUpstreamHit(t *testing.T) {
	hits := 0
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(origin.Close)

	relay := buildRelay(t, "tok", origin.URL)
	mux := newMux(relay)

	// 5 MiB + 1 byte — just over the limit
	oversizeBody := bytes.Repeat([]byte("x"), maxBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(oversizeBody))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rr.Code)
	}
	if hits != 0 {
		t.Errorf("upstream was hit %d time(s), want 0", hits)
	}
}

func TestTracesHandler_ExactMaxBody_Passes(t *testing.T) {
	origin, _ := newOriginServer(t, http.StatusOK)
	relay := buildRelay(t, "tok", origin.URL)
	mux := newMux(relay)

	exactBody := bytes.Repeat([]byte("x"), maxBodyBytes)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(exactBody))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for exact max body", rr.Code)
	}
}

// ── redirect: upstream redirects → relay returns redirect status (no follow) ──

func TestTracesHandler_RedirectNotFollowed(t *testing.T) {
	redirectHits := 0
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectHits++
		http.Redirect(w, r, "http://evil.example.com/steal", http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	relay := buildRelay(t, "tok", origin.URL)
	mux := newMux(relay)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("data")))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if redirectHits != 1 {
		t.Errorf("origin hit %d times, want exactly 1", redirectHits)
	}
	// Relay must propagate the 302 status, not follow to a second host.
	if rr.Code != http.StatusFound {
		t.Errorf("relay status = %d, want 302", rr.Code)
	}
}

// ── canceled request: relay uses r.Context() for upstream ────────────────────
//
// The relay passes r.Context() to http.NewRequestWithContext. This test injects
// a context that is already cancelled before the upstream call so the outgoing
// http.Client.Do returns context.Canceled immediately, meaning:
//   - The upstream server sees zero hits (the connection is never opened).
//   - The relay handler returns without writing a response body (graceful path).
//
// This is a unit-level proof of "context is wired" without requiring TCP timing.
func TestTracesHandler_ContextCanceled_UpstreamContextDone(t *testing.T) {
	hits := 0
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(origin.Close)

	relay := buildRelay(t, "tok", origin.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately — context is already done

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("data")))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	relay.tracesHandler(rr, req)

	// The already-cancelled context must prevent the upstream hit.
	if hits != 0 {
		t.Errorf("upstream was hit %d time(s), want 0 (context was pre-cancelled)", hits)
	}
}

// ── GET /v1/traces not allowed ────────────────────────────────────────────────

func TestTracesHandler_GETNotAllowed(t *testing.T) {
	mux := newMux(&server{token: "tok", targetURL: "http://unused", client: newClient()})
	req := httptest.NewRequest(http.MethodGet, "/v1/traces", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /v1/traces status = %d, want 405", rr.Code)
	}
}

// ── arbitrary paths return 404 ────────────────────────────────────────────────

func TestArbitraryPathsReturn404(t *testing.T) {
	mux := newMux(&server{token: "tok", targetURL: "http://unused", client: newClient()})
	paths := []string{"/", "/v1/metrics", "/admin", "/v1/traces/extra", "/health"}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("path %q: status = %d, want 404", p, rr.Code)
		}
	}
}

// ── upstream status propagated ────────────────────────────────────────────────

func TestTracesHandler_PropagatesUpstreamStatus(t *testing.T) {
	for _, wantStatus := range []int{200, 207, 400, 503} {
		origin, _ := newOriginServer(t, wantStatus)
		relay := buildRelay(t, "tok", origin.URL)
		mux := newMux(relay)

		req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("data")))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != wantStatus {
			t.Errorf("upstream %d → relay status %d, want %d", wantStatus, rr.Code, wantStatus)
		}
	}
}

// ── error body must not expose secrets ───────────────────────────────────────

func TestErrorBodyDoesNotExposeToken(t *testing.T) {
	token := "super-secret-bearer-value"
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo the Authorization header back in the body — relay must not forward this.
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("auth=" + r.Header.Get("Authorization")))
	}))
	t.Cleanup(origin.Close)

	relay := buildRelay(t, token, origin.URL)
	mux := newMux(relay)

	// Simulate a bad-gateway scenario by pointing to a closed port.
	closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedServer.Close()
	relayBad := &server{token: token, targetURL: closedServer.URL + "/v1/traces", client: newClient()}
	muxBad := newMux(relayBad)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("data")))
	rr := httptest.NewRecorder()
	muxBad.ServeHTTP(rr, req)

	if strings.Contains(rr.Body.String(), token) {
		t.Errorf("error response body exposed token: %q", rr.Body.String())
	}
	_ = mux // suppress unused warning
}

// ── both base + complete endpoint forwarding genuine handler test ───────────

func TestTracesHandler_BothBaseAndCompleteEndpointForwarding(t *testing.T) {
	for _, tc := range []struct {
		name       string
		isComplete bool
	}{
		{"base_url", false},
		{"complete_endpoint", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			var capturedPath, capturedAuth, capturedCT string
			var capturedBody []byte

			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				capturedPath = r.URL.Path
				capturedAuth = r.Header.Get("Authorization")
				capturedCT = r.Header.Get("Content-Type")
				b, _ := io.ReadAll(r.Body)
				capturedBody = b
				w.WriteHeader(http.StatusOK)
			}))
			defer origin.Close()

			endpoint := origin.URL
			if tc.isComplete {
				endpoint = origin.URL + "/v1/traces"
			}
			relay := buildRelay(t, "secret-bearer-tok", endpoint)
			mux := newMux(relay)

			payload := []byte("valid-binary-trace-record")
			req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/x-protobuf")

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if hits != 1 {
				t.Fatalf("upstream hits = %d, want 1", hits)
			}
			if capturedPath != "/v1/traces" {
				t.Errorf("upstream path = %q, want /v1/traces", capturedPath)
			}
			if capturedAuth != "Bearer secret-bearer-tok" {
				t.Errorf("upstream Authorization = %q, want 'Bearer secret-bearer-tok'", capturedAuth)
			}
			if capturedCT != "application/x-protobuf" {
				t.Errorf("upstream Content-Type = %q, want 'application/x-protobuf'", capturedCT)
			}
			if !bytes.Equal(capturedBody, payload) {
				t.Errorf("upstream body = %q, want %q", capturedBody, payload)
			}

			// Oversize body must return 413 and send ZERO hits upstream
			oversizeBody := bytes.Repeat([]byte("x"), maxBodyBytes+1)
			reqOver := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(oversizeBody))
			rrOver := httptest.NewRecorder()
			mux.ServeHTTP(rrOver, reqOver)

			if rrOver.Code != http.StatusRequestEntityTooLarge {
				t.Errorf("oversize status = %d, want 413", rrOver.Code)
			}
			if hits != 1 {
				t.Errorf("upstream hits after oversize = %d, want unchanged 1 (zero new hits)", hits)
			}
		})
	}
}

// ── health-before-token test ──────────────────────────────────────────────────

func TestHealthz_BeforeTokenAndNoTokenRequired(t *testing.T) {
	// GET /healthz must work without a valid token, without any file read, and
	// without any upstream contact.
	relay := &server{token: "", targetURL: "http://unreachable.invalid", client: newClient()}
	mux := newMux(relay)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ok") {
		t.Errorf("body = %q, want 'ok'", rr.Body.String())
	}
}

// ── token loader must not reflect secret in error messages ────────────────────

func TestLoadToken_DoesNotReflectSecretInError(t *testing.T) {
	secretVal := "secretTokenWithBadChar!!!"
	path := writeTokenFile(t, secretVal)
	_, err := loadToken(path)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if strings.Contains(err.Error(), secretVal) {
		t.Fatalf("loadToken leaked secret in error message: %v", err)
	}
}
