package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// extractClientIP extracts client IP address from HTTP headers and connection remote address.
func extractClientIP(header http.Header, remoteAddr string, trustForwarded bool) string {
	if trustForwarded && header != nil {
		if ip := header.Get("X-Real-IP"); ip != "" {
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
		if xff := header.Get("X-Forwarded-For"); xff != "" {
			for _, raw := range strings.Split(xff, ",") {
				ip := strings.TrimSpace(raw)
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
	}

	if ip, _, err := net.SplitHostPort(remoteAddr); err == nil {
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

// getClientIPWithTrust returns the client IP. When trustForwardedHeaders is
// true, X-Real-IP and the leftmost X-Forwarded-For entry are honoured (use
// only when behind a controlled reverse proxy that overwrites those headers).
// When false, headers are ignored and only the connection peer is returned —
// this prevents IP spoofing for rate-limit bypass when alt-backend is reached
// directly (M-007).
func getClientIPWithTrust(c echo.Context, trustForwardedHeaders bool) string {
	return extractClientIP(c.Request().Header, c.Request().RemoteAddr, trustForwardedHeaders)
}
