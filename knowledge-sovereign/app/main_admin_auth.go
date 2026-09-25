package main

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
)

func isAdminProtectedPath(path string) bool {
	return strings.HasPrefix(path, "/admin/") || path == "/health/deep"
}

func validateAdminBearerToken(authHeader, expectedToken string) bool {
	if expectedToken == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}
	provided := strings.TrimPrefix(authHeader, prefix)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expectedToken)) == 1
}

func logAdminAuthStatus(enabled bool) {
	if enabled {
		slog.Info("admin_auth_enabled")
	} else {
		slog.Warn("admin_auth_disabled: ADMIN_AUTH=disabled was set explicitly; /admin/* endpoints on the metrics port accept unauthenticated requests")
	}
}

// requireAdminToken wraps next so that /admin/* and /health/deep requests
// must carry "Authorization: Bearer <token>" matching the configured admin
// token. Cheap /health stays unauthenticated so compose probes keep working.
// Pass-through happens only when enabled is false, which config.Load grants
// solely for an explicit ADMIN_AUTH=disabled. An empty token with the gate on
// denies every request rather than opening the surface.
func requireAdminToken(token string, enabled bool, next http.Handler) http.Handler {
	if !enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAdminProtectedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !validateAdminBearerToken(r.Header.Get("Authorization"), token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
