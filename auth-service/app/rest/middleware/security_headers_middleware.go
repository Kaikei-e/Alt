package middleware

import (
	"github.com/labstack/echo/v4"
)

func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// セキュリティヘッダーの設定
			headers := c.Response().Header()

			// HTTPS強制
			headers.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

			// XSS保護
			headers.Set("X-XSS-Protection", "1; mode=block")

			// MIME Type Sniffing防止
			headers.Set("X-Content-Type-Options", "nosniff")

			// Clickjacking防止
			headers.Set("X-Frame-Options", "DENY")

			// Referrer Policy
			headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy
			csp := "default-src 'self'; " +
				"script-src 'self' 'unsafe-inline'; " +
				"style-src 'self' 'unsafe-inline'; " +
				"img-src 'self' data: https:; " +
				"connect-src 'self' https://kratos-public.alt-auth.svc.cluster.local; " +
				"font-src 'self'; " +
				"object-src 'none'; " +
				"base-uri 'self'; " +
				"form-action 'self'"
			headers.Set("Content-Security-Policy", csp)

			// Permissions Policy
			permissions := "geolocation=(), microphone=(), camera=(), payment=(), usb=()"
			headers.Set("Permissions-Policy", permissions)

			return next(c)
		}
	}
}