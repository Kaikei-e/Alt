package handler

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/middleware"
)

// RESTProxyHandler proxies REST API requests to the backend.
// It validates JWT tokens and forwards requests transparently.
// No BFF features (cache, circuit breaker, dedup) are applied.
type RESTProxyHandler struct {
	backendClient   *client.BackendClient
	authInterceptor *middleware.AuthInterceptor
	logger          *slog.Logger
	requestTimeout  time.Duration
}

// NewRESTProxyHandler creates a new REST proxy handler.
func NewRESTProxyHandler(
	backendClient *client.BackendClient,
	secret []byte,
	issuer, audience string,
	logger *slog.Logger,
	requestTimeout time.Duration,
) *RESTProxyHandler {
	return &RESTProxyHandler{
		backendClient:   backendClient,
		authInterceptor: middleware.NewAuthInterceptor(logger, secret, issuer, audience),
		logger:          logger,
		requestTimeout:  requestTimeout,
	}
}

// ServeHTTP implements http.Handler.
func (h *RESTProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract and validate JWT token
	token := r.Header.Get(middleware.BackendTokenHeader)
	_, err := h.authInterceptor.ValidateToken(token)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("REST proxy auth failed", "error", err, "path", r.URL.Path)
		}
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Forward request to backend
	resp, err := h.backendClient.ForwardRESTRequest(r, token)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("REST proxy backend request failed", "error", err, "path", r.URL.Path)
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy all response headers
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Stream response body
	io.Copy(w, resp.Body)
}
