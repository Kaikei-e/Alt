// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/middleware"
)

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
	backendClient   *client.BackendClient
	authInterceptor *middleware.AuthInterceptor
	logger          *slog.Logger
}

// NewProxyHandler creates a new proxy handler.
func NewProxyHandler(
	backendClient *client.BackendClient,
	secret []byte,
	issuer, audience string,
	logger *slog.Logger,
) *ProxyHandler {
	return &ProxyHandler{
		backendClient:   backendClient,
		authInterceptor: middleware.NewAuthInterceptor(logger, secret, issuer, audience),
		logger:          logger,
	}
}

// ServeHTTP implements http.Handler.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
