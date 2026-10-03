// Package main implements a private OTLP HTTP relay that forwards POST /v1/traces
// requests to a downstream collector, injecting a Bearer token loaded once at startup.
// Only POST /v1/traces and GET /healthz are served; all other paths return 404.
package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	maxBodyBytes = 5 * 1024 * 1024 // 5 MiB
	listenAddr   = "0.0.0.0:4318"
	healthAddr   = "http://127.0.0.1:4318/healthz"
)

// isValidToken checks that s is a non-empty RFC 7235 token68 value:
// characters in [A-Za-z0-9._~+/-] with an optional trailing sequence of '='.
// token68  = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="
func isValidToken(s string) bool {
	if s == "" {
		return false
	}
	seenEq := false
	hasBaseChar := false
	for _, c := range s {
		switch {
		case c == '=':
			if !hasBaseChar {
				// Require at least one valid alphabet/digit/symbol char before padding
				return false
			}
			seenEq = true
		case seenEq:
			// '=' must only appear at the end
			return false
		case (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' ||
			c == '+' || c == '/':
			hasBaseChar = true
		default:
			return false
		}
	}
	return hasBaseChar
}

// parseAndNormalizeTargetURL returns the canonical string if raw is a valid http/https URL with
// host, no userinfo, no query, and no fragment. Path must be either empty, "/",
// or canonical fullpath "/v1/traces" (with optional trailing slash).
func parseAndNormalizeTargetURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT must not be empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT: invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT: scheme must be http or https")
	}
	if u.Hostname() == "" {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT: host must not be empty")
	}
	if u.User != nil {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT: userinfo not allowed")
	}
	if u.RawQuery != "" || u.ForceQuery {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT: query not allowed")
	}
	if u.Fragment != "" {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT: fragment not allowed")
	}
	if u.RawPath != "" && u.RawPath != u.Path {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT: encoded path not allowed")
	}
	cleanPath := strings.TrimSuffix(u.Path, "/")
	if cleanPath != "" && cleanPath != "/v1/traces" {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT: ambiguous path; must be base URL or /v1/traces")
	}
	u.Path = "/v1/traces"
	u.RawPath = ""
	return u.String(), nil
}

// loadToken reads the token file, trims whitespace, and validates it.
func loadToken(path string) (string, error) {
	if path == "" {
		return "", errors.New("RASK_INGEST_TOKEN_FILE must be set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("RASK_INGEST_TOKEN_FILE: cannot read file")
	}
	tok := strings.TrimRight(string(raw), "\r\n")
	// Strip a single trailing newline to be friendly with Docker secrets format.
	if !isValidToken(tok) {
		return "", errors.New("RASK_INGEST_TOKEN_FILE: token does not satisfy RFC 7235 token68 charset")
	}
	return tok, nil
}

// newClient returns an http.Client configured with a 5-second timeout,
// redirect policy of ErrUseLastResponse, and verified TLS (stdlib defaults).
func newClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// server encapsulates the relay handler dependencies.
type server struct {
	token     string
	targetURL string
	client    *http.Client
}

// healthzHandler serves GET /healthz. No upstream call; no exports.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok\n")) //nolint:errcheck
}

// tracesHandler handles POST /v1/traces.
func (s *server) tracesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read up to maxBodyBytes+1 to detect oversize before touching upstream.
	limited := io.LimitReader(r.Body, maxBodyBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		// Context cancelled or connection reset — propagate cleanly.
		if errors.Is(err, context.Canceled) || r.Context().Err() != nil {
			return
		}
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if int64(len(bodyBytes)) > maxBodyBytes {
		http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
		return
	}

	ct := r.Header.Get("Content-Type")

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Only forward Content-Type. Never forward incoming Authorization.
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	// Inject relay's own Bearer token — never echo the caller's credentials.
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		if r.Context().Err() != nil {
			// Client cancelled — do not write; just return.
			return
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Propagate upstream status + body. Do not expose upstream error details.
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}

// newMux wires routes: only POST /v1/traces and GET /healthz are handled.
func newMux(srv *server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/v1/traces", srv.tracesHandler)
	return mux
}

func main() {
	// Health-check subcommand: used by Dockerfile HEALTHCHECK / liveness probe.
	// Deliberately skips Bearer startup — only needs the relay port to be reachable.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		c := &http.Client{Timeout: 2 * time.Second}
		resp, err := c.Get(healthAddr)
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// ── Startup validation ────────────────────────────────────────────────────

	endpointEnv := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpointEnv == "" {
		endpointEnv = "http://rask-log-aggregator:4318"
	}
	targetURL, err := parseAndNormalizeTargetURL(endpointEnv)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	tok, err := loadToken(os.Getenv("RASK_INGEST_TOKEN_FILE"))
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	// ── HTTP server ───────────────────────────────────────────────────────────

	s := &server{
		token:     tok,
		targetURL: targetURL,
		client:    newClient(),
	}

	httpSrv := &http.Server{
		Addr:         listenAddr,
		Handler:      newMux(s),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		log.Printf("otel-relay listening on %s → %s", listenAddr, targetURL)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
