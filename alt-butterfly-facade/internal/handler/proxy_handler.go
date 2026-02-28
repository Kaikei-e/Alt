// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/middleware"
)

// maxConnectTimeout is the upper bound for Connect-Timeout-Ms to prevent abuse.
const maxConnectTimeout = 5 * time.Minute

// streamingProcedures lists Connect-RPC procedures that use server streaming
var streamingProcedures = map[string]bool{
	"/alt.feeds.v2.FeedService/StreamFeedStats":              true,
	"/alt.feeds.v2.FeedService/StreamSummarize":              true,
	"/alt.augur.v2.AugurService/StreamChat":                  true,
	"/alt.morning_letter.v2.MorningLetterService/StreamChat": true,
	"/alt.tts.v1.TTSService/SynthesizeStream":                true,
}

// ProxyHandler proxies Connect-RPC requests to the backend.
type ProxyHandler struct {
	backendClient    *client.BackendClient
	authInterceptor  *middleware.AuthInterceptor
	logger           *slog.Logger
	defaultTimeout   time.Duration
	streamingTimeout time.Duration
}

// NewProxyHandler creates a new proxy handler.
func NewProxyHandler(
	backendClient *client.BackendClient,
	secret []byte,
	issuer, audience string,
	logger *slog.Logger,
	defaultTimeout time.Duration,
	streamingTimeout time.Duration,
) *ProxyHandler {
	return &ProxyHandler{
		backendClient:    backendClient,
		authInterceptor:  middleware.NewAuthInterceptor(logger, secret, issuer, audience),
		logger:           logger,
		defaultTimeout:   defaultTimeout,
		streamingTimeout: streamingTimeout,
	}
}

// applyConnectTimeout parses the Connect-Timeout-Ms header and sets a context
// deadline on the request. If the header is absent or invalid, defaultTimeout
// is used (or streamingTimeout for streaming procedures).
// The timeout is capped at maxConnectTimeout (5 min).
func (h *ProxyHandler) applyConnectTimeout(r *http.Request) (*http.Request, context.CancelFunc) {
	timeout := h.defaultTimeout

	// Use streaming timeout for streaming procedures
	if isStreamingProcedure(r.URL.Path) {
		timeout = h.streamingTimeout
	}

	if ms := r.Header.Get("Connect-Timeout-Ms"); ms != "" {
		if parsed, err := strconv.ParseInt(ms, 10, 64); err == nil && parsed > 0 {
			timeout = time.Duration(parsed) * time.Millisecond
		}
	}

	if timeout > maxConnectTimeout {
		timeout = maxConnectTimeout
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	return r.WithContext(ctx), cancel
}

// ServeHTTP implements http.Handler.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Apply Connect-Timeout-Ms based context deadline
	r, cancel := h.applyConnectTimeout(r)
	defer cancel()

	// Extract and validate token
	token := r.Header.Get(middleware.BackendTokenHeader)
	_, err := h.authInterceptor.ValidateToken(token)
	if err != nil {
		h.logError("authentication failed", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Determine if this is a streaming request
	isStreaming := isStreamingProcedure(r.URL.Path)

	var resp *http.Response
	if isStreaming {
		resp, err = h.backendClient.ForwardStreamingRequest(r, token)
	} else {
		resp, err = h.backendClient.ForwardRequest(r, token)
	}

	if err != nil {
		h.logError("backend request failed", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	copyResponseHeaders(resp.Header, w.Header())

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	if isStreaming {
		h.streamResponse(w, resp)
	} else {
		io.Copy(w, resp.Body)
	}
}

// streamResponse handles streaming response copying with flushing.
func (h *ProxyHandler) streamResponse(w http.ResponseWriter, resp *http.Response) {
	flusher, canFlush := w.(http.Flusher)

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}

// isStreamingProcedure checks if a procedure uses server streaming.
func isStreamingProcedure(path string) bool {
	// Normalize path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return streamingProcedures[path]
}

// copyResponseHeaders copies relevant headers from backend response.
func copyResponseHeaders(src, dst http.Header) {
	headersToForward := []string{
		"Content-Type",
		"Grpc-Status",
		"Grpc-Message",
		"Connect-Content-Encoding",
		"Connect-Accept-Encoding",
		"Trailer",
	}

	for _, h := range headersToForward {
		if v := src.Get(h); v != "" {
			dst.Set(h, v)
		}
	}
}

// logError logs an error with context.
func (h *ProxyHandler) logError(msg string, err error) {
	if h.logger != nil {
		h.logger.Error(msg, "error", err)
	}
}
