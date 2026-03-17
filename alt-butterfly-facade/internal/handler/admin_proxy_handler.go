package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/middleware"
)

// AdminProxyHandler verifies admin access using the caller's backend token,
// then forwards the request to alt-backend using service-token auth.
type AdminProxyHandler struct {
	backendClient   *client.BackendClient
	authInterceptor *middleware.AuthInterceptor
	serviceSecret   string
	logger          *slog.Logger
	defaultTimeout  time.Duration
}

func NewAdminProxyHandler(
	backendClient *client.BackendClient,
	secret []byte,
	issuer, audience string,
	serviceSecret string,
	logger *slog.Logger,
	defaultTimeout time.Duration,
) *AdminProxyHandler {
	return &AdminProxyHandler{
		backendClient:   backendClient,
		authInterceptor: middleware.NewAuthInterceptor(logger, secret, issuer, audience),
		serviceSecret:   serviceSecret,
		logger:          logger,
		defaultTimeout:  defaultTimeout,
	}
}

func (h *AdminProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.defaultTimeout)
	defer cancel()
	r = r.WithContext(ctx)

	token := r.Header.Get(middleware.BackendTokenHeader)
	userCtx, err := h.authInterceptor.ValidateToken(token)
	if err != nil {
		h.logError("admin proxy authentication failed", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if string(userCtx.Role) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	resp, err := h.backendClient.ForwardServiceRequest(r, h.serviceSecret)
	if err != nil {
		h.logError("admin proxy backend request failed", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(resp.Header, w.Header())
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *AdminProxyHandler) logError(msg string, err error) {
	if h.logger != nil {
		h.logger.Error(msg, "error", err)
	}
}
