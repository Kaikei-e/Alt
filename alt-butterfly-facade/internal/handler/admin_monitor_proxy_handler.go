package handler

import (
	"io"
	"log/slog"
	"net/http"

	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/middleware"
)

// AdminMonitorProxyHandler relays Connect-RPC calls to
// alt.admin_monitor.v1.AdminMonitorService. It differs from AdminProxyHandler
// in that it does not apply a unary request timeout: Watch is a long-lived
// server stream that should only terminate on client cancel or the configured
// streaming ceiling (handled by the backend client's streaming timeout, which
// defaults to 30 min).
//
// Contract:
//   - caller must present an X-Alt-Backend-Token with role=admin
//   - outbound request carries TLS peer identity only
//   - streaming headers (X-Accel-Buffering, Content-Type) are propagated
//   - response chunks are flushed as soon as they arrive (io.Copy +
//     http.Flusher) so intermediate proxies cannot batch them
type AdminMonitorProxyHandler struct {
	backendClient   *client.BackendClient
	authInterceptor *middleware.AuthInterceptor
	serviceSecret   string
	logger          *slog.Logger
}

func NewAdminMonitorProxyHandler(
	backendClient *client.BackendClient,
	secret []byte,
	issuer, audience string,
	serviceSecret string,
	logger *slog.Logger,
) *AdminMonitorProxyHandler {
	return &AdminMonitorProxyHandler{
		backendClient:   backendClient,
		authInterceptor: middleware.NewAuthInterceptor(logger, secret, issuer, audience),
		serviceSecret:   serviceSecret,
		logger:          logger,
	}
}

func (h *AdminMonitorProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get(middleware.BackendTokenHeader)
	userCtx, err := h.authInterceptor.ValidateToken(token)
	if err != nil {
		h.logError("admin monitor proxy authentication failed", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if string(userCtx.Role) != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Use the streaming client path: no short context timeout is applied so
	// Watch can run for the full stream lifetime. Unary Snapshot/Catalog
	// finishes quickly and is unaffected.
	resp, err := h.backendClient.ForwardServiceRequest(r, h.serviceSecret)
	if err != nil {
		h.logError("admin monitor proxy backend request failed", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(resp.Header, w.Header())
	// Defense in depth: always disable intermediate buffering for admin monitor.
	if w.Header().Get("X-Accel-Buffering") == "" {
		w.Header().Set("X-Accel-Buffering", "no")
	}
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	if flusher == nil {
		_, _ = io.Copy(w, resp.Body)
		return
	}
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			flusher.Flush()
		}
		if readErr != nil {
			return
		}
	}
}

func (h *AdminMonitorProxyHandler) logError(msg string, err error) {
	if h.logger != nil {
		h.logger.Error(msg, "error", err)
	}
}
