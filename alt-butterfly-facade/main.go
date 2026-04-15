package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alt-butterfly-facade/config"
	"alt-butterfly-facade/internal/logger"
	"alt-butterfly-facade/internal/server"
	"alt-butterfly-facade/internal/tlsutil"

	"golang.org/x/net/http2"
)

// newMTLSBackendTransport builds an HTTP/2 RoundTripper that presents the
// alt-butterfly-facade leaf cert on every handshake. The transport is used
// for every mTLS upstream (alt-backend, acolyte-orchestrator, tts-speaker
// nginx sidecars). ServerName is intentionally left empty so the HTTP
// client derives SNI from each request's URL host — required because the
// same transport is reused across multiple internal hostnames.
func newMTLSBackendTransport() (http.RoundTripper, error) {
	tlsCfg, err := tlsutil.LoadClientConfig(
		os.Getenv("MTLS_CERT_FILE"),
		os.Getenv("MTLS_KEY_FILE"),
		os.Getenv("MTLS_CA_FILE"),
	)
	if err != nil {
		return nil, err
	}
	return &http2.Transport{
		TLSClientConfig: tlsCfg,
	}, nil
}

func main() {
	// Handle healthcheck subcommand (for Docker healthcheck in distroless image)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "Healthcheck failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	ctx := context.Background()

	// Initialize structured logger with trace context support
	appLogger := logger.Init()

	// Load configuration
	cfg := config.NewConfig()
	if err := cfg.Validate(); err != nil {
		slog.ErrorContext(ctx, "invalid configuration", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "configuration loaded",
		"port", cfg.Port,
		"backend_url", cfg.BackendConnectURL,
		"backend_rest_url", cfg.BackendRESTURL,
		"tts_url", cfg.TTSConnectURL,
		"acolyte_url", cfg.AcolyteConnectURL,
		"issuer", cfg.BackendTokenIssuer,
		"audience", cfg.BackendTokenAudience)

	// Load backend token secret
	secret, err := cfg.LoadBackendTokenSecret()
	if err != nil {
		slog.ErrorContext(ctx, "failed to load backend token secret", "error", err)
		os.Exit(1)
	}

	// Service-to-service auth is established at the TLS transport layer.

	backendURL := cfg.BackendConnectURL
	acolyteURL := cfg.AcolyteConnectURL
	ttsURL := cfg.TTSConnectURL
	var backendTransport http.RoundTripper
	if os.Getenv("MTLS_ENFORCE") == "true" {
		backendTransport, err = newMTLSBackendTransport()
		if err != nil {
			slog.ErrorContext(ctx, "backend mTLS transport (fail-closed)", "error", err)
			os.Exit(1)
		}
		if v := os.Getenv("BACKEND_CONNECT_MTLS_URL"); v != "" {
			backendURL = v
		}
		// Acolyte and TTS each expose their own nginx mTLS sidecar on :9443.
		// When MTLS_ENFORCE is on, route BFF → Acolyte/TTS through those TLS
		// listeners so the whole east-west fabric stays off plaintext.
		if v := os.Getenv("ACOLYTE_CONNECT_MTLS_URL"); v != "" {
			acolyteURL = v
		}
		if v := os.Getenv("TTS_CONNECT_MTLS_URL"); v != "" {
			ttsURL = v
		}
		slog.InfoContext(ctx, "BFF outbound clients: mTLS enforce enabled",
			"backend", backendURL, "acolyte", acolyteURL, "tts", ttsURL)
	}

	// Create server configuration
	serverCfg := server.Config{
		BackendURL:        backendURL,
		BackendRESTURL:    cfg.BackendRESTURL,
		Secret:            secret,
		Issuer:            cfg.BackendTokenIssuer,
		Audience:          cfg.BackendTokenAudience,
		RequestTimeout:    cfg.RequestTimeout,
		StreamingTimeout:  cfg.StreamingTimeout,
		TTSConnectURL:     ttsURL,
		AcolyteConnectURL: acolyteURL,
	}

	// Connect-RPC uses the mTLS transport when enforcement is on; REST
	// proxies always stay on the default plaintext transport so that
	// alt-backend's Echo listener on :9000 keeps serving OPML, dashboard,
	// admin scraping and image proxy routes without TLS scheme conflicts.
	handler := server.NewServerWithTransports(
		serverCfg,
		appLogger,
		backendTransport,
		nil,
	)

	address := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         address,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: cfg.StreamingTimeout + 10*time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		slog.InfoContext(ctx, "starting alt-butterfly-facade server", "address", address)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Optional mTLS HTTPS listener mirroring the h2c handler.
	// ClientAuth defaults to NoClientCert; enabled by MTLS_LISTEN=true.
	var mtlsServer *http.Server
	if os.Getenv("MTLS_LISTEN") == "true" {
		mtlsPort := os.Getenv("MTLS_PORT")
		if mtlsPort == "" {
			mtlsPort = "9443"
		}
		tlsCfg, err := tlsutil.LoadServerConfig(
			os.Getenv("MTLS_CERT_FILE"),
			os.Getenv("MTLS_KEY_FILE"),
			os.Getenv("MTLS_CA_FILE"),
			tlsutil.OptionsFromEnv()...,
		)
		if err != nil {
			slog.ErrorContext(ctx, "mTLS listener config failed, aborting startup (fail-closed)", "error", err)
			os.Exit(1)
		}
		{
			mtlsServer = tlsutil.NewMTLSHTTPServer(":"+mtlsPort, tlsCfg, handler)
			go func() {
				slog.InfoContext(ctx, "mTLS HTTPS listener starting", "port", mtlsPort)
				if err := mtlsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					slog.ErrorContext(ctx, "mTLS HTTPS listener error", "error", err)
				}
			}()
		}
	}

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.InfoContext(ctx, "shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if mtlsServer != nil {
		_ = mtlsServer.Shutdown(shutdownCtx)
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(ctx, "server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "server exited properly")
}

// runHealthcheck performs a health check against the local server
func runHealthcheck() error {
	port := os.Getenv("BFF_PORT")
	if port == "" {
		port = "9200"
	}

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned status: %d", resp.StatusCode)
	}

	return nil
}
