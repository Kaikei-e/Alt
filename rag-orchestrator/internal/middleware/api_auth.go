package middleware

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	apiAuthHeader = "Authorization"
	bearerPrefix  = "Bearer "
)

// APIAuthMiddleware enforces caller authentication on rag-orchestrator's
// plaintext listener (:9010). It requires a bearer token on every request,
// ensuring identity headers such as X-Alt-User-Id are only trusted behind
// an authenticated caller.
//
// When enabled is false (RAG_API_AUTH=disabled), requests pass through
// without authentication.
type APIAuthMiddleware struct {
	token   string
	enabled bool
	logger  *slog.Logger
}

// NewAPIAuthMiddleware creates the API authentication middleware.
func NewAPIAuthMiddleware(token string, enabled bool, logger *slog.Logger) *APIAuthMiddleware {
	if logger == nil {
		logger = slog.Default()
	}
	return &APIAuthMiddleware{
		token:   token,
		enabled: enabled,
		logger:  logger,
	}
}

// isExemptPath returns true for health and observability endpoints that
// do not require authentication.
func isExemptPath(path string) bool {
	if path == "/metrics" {
		return true
	}
	if path == "/healthz" || path == "/readyz" {
		return true
	}
	if path == "/health" || strings.HasPrefix(path, "/health/") {
		return true
	}
	return false
}

func (m *APIAuthMiddleware) checkAuth(authHeader string) bool {
	if !m.enabled {
		return true
	}
	if m.token == "" {
		return false
	}
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return false
	}
	presented := strings.TrimPrefix(authHeader, bearerPrefix)
	return subtle.ConstantTimeCompare([]byte(presented), []byte(m.token)) == 1
}

// EchoMiddleware returns an Echo middleware function gating routes on the :9010 REST server.
func (m *APIAuthMiddleware) EchoMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if isExemptPath(c.Request().URL.Path) {
				return next(c)
			}

			auth := c.Request().Header.Get(apiAuthHeader)
			if !m.checkAuth(auth) {
				m.logger.WarnContext(c.Request().Context(), "rag_api_auth_unauthorized",
					"path", c.Request().URL.Path,
					"has_header", auth != "",
				)
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			return next(c)
		}
	}
}

// WrapHandler wraps standard HTTP and Connect handlers with bearer token authentication.
func (m *APIAuthMiddleware) WrapHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isExemptPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get(apiAuthHeader)
		if !m.checkAuth(auth) {
			m.logger.WarnContext(r.Context(), "rag_api_auth_unauthorized",
				"path", r.URL.Path,
				"has_header", auth != "",
			)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
