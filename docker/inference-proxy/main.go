package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultUpstreamResponseHeaderTimeout is the default time to wait for the
// upstream to send its first response header. 600s accommodates cold LLM
// first-token latency without imposing a hard deadline on long streams.
const DefaultUpstreamResponseHeaderTimeout = 600 * time.Second

// DefaultMaxRequestBodyBytes is the default maximum request body size in bytes.
// Requests exceeding this limit are rejected with HTTP 413 before the upstream
// receives a single byte — the upstream hit count stays at zero.
const DefaultMaxRequestBodyBytes int64 = 16 << 20 // 16 MiB

// DefaultReadTimeout is the default maximum duration for reading the entire
// request (including body) before the server terminates the connection.
// Prevents slow-upload / Slowloris DoS while leaving WriteTimeout 0 for
// long streaming responses.
const DefaultReadTimeout = 30 * time.Second

// DefaultReadHeaderTimeout is the maximum duration allowed to read request headers.
const DefaultReadHeaderTimeout = 10 * time.Second

// NewProxyHandler builds the HTTP handler for the inference auth proxy.
//
// maxBodyBytes is the maximum allowed request body size. The entire body is
// drained (up to maxBodyBytes+1) before any upstream contact; a body that
// exceeds the limit returns 413 immediately and the upstream hit count is zero.
//
// responseHeaderTimeout is the time to wait for the upstream's first response
// header; ≤0 falls back to DefaultUpstreamResponseHeaderTimeout (600s).
func NewProxyHandler(targetURL *url.URL, expectedToken string, responseHeaderTimeout time.Duration, maxBodyBytes int64) http.Handler {
	if responseHeaderTimeout <= 0 {
		responseHeaderTimeout = DefaultUpstreamResponseHeaderTimeout
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = DefaultMaxRequestBodyBytes
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  responseHeaderTimeout,
		MaxResponseHeaderBytes: 1 << 20, // 1 MiB header limit
		MaxIdleConns:           100,
		MaxIdleConnsPerHost:    10,
		IdleConnTimeout:        90 * time.Second,
		ForceAttemptHTTP2:      true,
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = transport
	proxy.FlushInterval = -1 // Immediate flush for streaming responses
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host // Required for some upstreams
		// Strip Authorization header before forwarding to upstream.
		// Service credentials must not be leaked to the raw upstream handler.
		req.Header.Del("Authorization")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		providedToken := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(expectedToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Enforce bounded request body size and read integrity BEFORE forwarding to upstream.
		// Using io.ReadAll on io.LimitReader(r.Body, maxBodyBytes+1) bounds memory allocation
		// to at most max+1 bytes. Read errors (premature EOF, malformed/truncated chunked bodies)
		// are rejected before contacting upstream; upstream hit count stays at zero.
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		_ = r.Body.Close()
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if int64(len(bodyBytes)) > maxBodyBytes {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Valid token and complete body within limit — forward to upstream.
		proxy.ServeHTTP(w, r)
	})

	return mux
}

// NewServer constructs the http.Server for the proxy.
// WriteTimeout is explicitly 0 (unbounded): prevents killing slow LLM streaming responses.
// ReadHeaderTimeout is finite (10s): prevents slowloris header exhaustion.
// ReadTimeout is finite (default 30s): prevents slow-upload DoS attacks while the body
// is pre-read into bounded memory.
func NewServer(addr string, handler http.Handler, readTimeout ...time.Duration) *http.Server {
	rt := DefaultReadTimeout
	if len(readTimeout) > 0 && readTimeout[0] > 0 {
		rt = readTimeout[0]
	}
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		ReadTimeout:       rt,
		WriteTimeout:      0, // Unbounded write timeout for streaming LLM responses
		IdleTimeout:       120 * time.Second,
	}
}

func main() {
	if isHealthcheckCommand(os.Args) {
		port := extractHealthcheckPort(os.Args)
		os.Exit(runHealthcheck(port, nil))
	}

	target := flag.String("target", "http://ollama:11434", "Upstream Ollama URL")
	port := flag.Int("port", 11436, "Port to listen on")
	tokenFile := flag.String("token-file", "/run/secrets/inference_token", "Path to the bearer token file")
	responseHeaderTimeout := flag.Duration("response-header-timeout", DefaultUpstreamResponseHeaderTimeout,
		"Time to wait for upstream first response header (default 600s accommodates cold LLM starts)")
	readTimeout := flag.Duration("read-timeout", DefaultReadTimeout,
		"Maximum duration for reading request body (default 30s; mitigates slow-upload DoS)")
	maxBodyMB := flag.Int("max-body-mb", int(DefaultMaxRequestBodyBytes>>20),
		"Maximum request body size in MiB; requests larger than this are rejected with 413 before upstream contact")
	flag.Parse()

	if *port <= 0 || *port > 65535 {
		log.Fatalf("Invalid port: must be between 1 and 65535, got %d", *port)
	}
	if *responseHeaderTimeout <= 0 || *responseHeaderTimeout > 3600*time.Second {
		log.Fatalf("Invalid response-header-timeout: must be positive and <= 3600s, got %v", *responseHeaderTimeout)
	}
	if *readTimeout <= 0 || *readTimeout > 300*time.Second {
		log.Fatalf("Invalid read-timeout: must be positive and <= 300s, got %v", *readTimeout)
	}
	if *maxBodyMB <= 0 || *maxBodyMB > 1024 {
		log.Fatalf("Invalid max-body-mb: must be positive and <= 1024, got %d", *maxBodyMB)
	}

	targetURL, err := url.Parse(*target)
	if err != nil {
		log.Fatalf("Invalid target URL: %v", err)
	}

	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		log.Fatalf("Strict upstream scheme required, got %s", targetURL.Scheme)
	}

	// Read expected token on startup — fail fast if missing or empty.
	expectedRaw, err := os.ReadFile(*tokenFile)
	if err != nil {
		log.Fatalf("Failed to read token file on startup: %v", err)
	}
	expectedToken := string(bytes.TrimSpace(expectedRaw))
	if expectedToken == "" {
		log.Fatalf("Token file is empty")
	}

	// Validate ASCII token (RFC 6750 safe).
	for i := 0; i < len(expectedToken); i++ {
		if expectedToken[i] < 32 || expectedToken[i] > 126 {
			log.Fatalf("Token contains invalid non-printable characters")
		}
	}

	maxBodyBytes := int64(*maxBodyMB) << 20
	handler := NewProxyHandler(targetURL, expectedToken, *responseHeaderTimeout, maxBodyBytes)
	server := NewServer(fmt.Sprintf(":%d", *port), handler, *readTimeout)

	log.Printf("Inference Auth Proxy listening on %s, routing to %s (response-header-timeout=%v, read-timeout=%v, max-body=%dMiB)",
		server.Addr, *target, *responseHeaderTimeout, *readTimeout, *maxBodyMB)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

// isHealthcheckCommand returns true if "healthcheck" is present in args.
func isHealthcheckCommand(args []string) bool {
	for i := 1; i < len(args); i++ {
		if args[i] == "healthcheck" {
			return true
		}
	}
	return false
}

// extractHealthcheckPort determines the healthcheck port from CLI args, environment, or default "11436".
func extractHealthcheckPort(args []string) string {
	for i := 1; i < len(args); i++ {
		if args[i] == "healthcheck" {
			for j := i + 1; j < len(args); j++ {
				arg := args[j]
				if (arg == "-port" || arg == "--port") && j+1 < len(args) {
					return args[j+1]
				}
				if strings.HasPrefix(arg, "-port=") {
					return strings.TrimPrefix(arg, "-port=")
				}
				if strings.HasPrefix(arg, "--port=") {
					return strings.TrimPrefix(arg, "--port=")
				}
				if !strings.HasPrefix(arg, "-") && arg != "" {
					return arg
				}
			}
			break
		}
	}
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	if addr := os.Getenv("LISTEN_ADDR"); addr != "" {
		if idx := strings.LastIndex(addr, ":"); idx >= 0 {
			return addr[idx+1:]
		}
	}
	return "11436"
}

// validateHealthPort ensures the port is strictly decimal digits in range 1..65535,
// preventing userinfo or host manipulation before any URL is built.
func validateHealthPort(portStr string) (int, error) {
	if portStr == "" {
		return 0, errors.New("port cannot be empty")
	}
	for i := 0; i < len(portStr); i++ {
		if portStr[i] < '0' || portStr[i] > '9' {
			return 0, fmt.Errorf("invalid port character %q: strict decimal digits required", portStr[i])
		}
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port: %w", err)
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("port out of range 1..65535: %d", p)
	}
	return p, nil
}

// runHealthcheck performs a bounded HTTP GET against the inference-proxy health endpoint.
// It checks loopback only with a 5s maximum timeout, requires HTTP 200, reads a bounded body (max 4KiB,
// failing closed if oversize), does not follow redirects, and ensures the body is closed.
// Returns 0 on success, non-zero on error.
func runHealthcheck(port string, client *http.Client) int {
	portNum, err := validateHealthPort(port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}

	var httpClient http.Client
	if client != nil {
		httpClient = *client // shallow clone to prevent mutating caller's pointer
	}
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if httpClient.Timeout <= 0 || httpClient.Timeout > 5*time.Second {
		httpClient.Timeout = 5 * time.Second
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/health", portNum)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck request build failed: %v\n", err)
		return 1
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	// Body bounded to max 4KiB (4096 bytes); oversize fails closed.
	const maxHealthBodyBytes int64 = 4096
	lr := io.LimitReader(resp.Body, maxHealthBodyBytes+1)
	n, _ := io.Copy(io.Discard, lr)
	if n > maxHealthBodyBytes {
		fmt.Fprintf(os.Stderr, "healthcheck failed: response body exceeded limit of %d bytes\n", maxHealthBodyBytes)
		return 1
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
