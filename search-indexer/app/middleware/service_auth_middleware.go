package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"search-indexer/logger"
)

const serviceTokenHeader = "X-Service-Token"

// ServiceAuthMiddleware enforces X-Service-Token on internal HTTP endpoints.
// It mirrors the contract established by ADR-000717 for alt-backend's
// /v1/internal/* routes: internal callers authenticate with a shared secret,
// compared in constant time.
type ServiceAuthMiddleware struct {
	serviceSecret string
}

// NewServiceAuthMiddleware constructs a middleware bound to the provided
// shared secret. An empty secret causes all requests to be rejected
// (fail-closed), so missing configuration never exposes endpoints.
func NewServiceAuthMiddleware(serviceSecret string) *ServiceAuthMiddleware {
	return &ServiceAuthMiddleware{serviceSecret: serviceSecret}
}

// RequireServiceAuth wraps the given handler, rejecting requests without a
// valid X-Service-Token header.
func (m *ServiceAuthMiddleware) RequireServiceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.serviceSecret == "" {
			logger.Logger.ErrorContext(r.Context(),
				"service auth misconfigured: SERVICE_TOKEN is empty")
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		token := strings.TrimSpace(r.Header.Get(serviceTokenHeader))
		if token == "" {
			logger.Logger.WarnContext(r.Context(),
				"service auth failed: missing token",
				"path", r.URL.Path, "remote_addr", r.RemoteAddr)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		expected := []byte(m.serviceSecret)
		provided := []byte(token)
		valid := len(expected) == len(provided) &&
			subtle.ConstantTimeCompare(expected, provided) == 1
		if !valid {
			logger.Logger.WarnContext(r.Context(),
				"service auth failed: invalid token",
				"path", r.URL.Path, "remote_addr", r.RemoteAddr)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
