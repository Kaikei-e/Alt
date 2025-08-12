package middleware

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge          int
}

// DefaultCORSConfig returns default CORS configuration
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{
			"http://localhost:3000",
			"https://alt.local",
			"https://*.alt.local",
			"http://alt-frontend.alt-apps.svc.cluster.local:3000",
			"https://alt-frontend.alt-apps.svc.cluster.local:3000",
			"http://alt.example.com",
			"https://alt.example.com",
			"https://app.alt.example.com",
		},
		AllowMethods: []string{
			echo.GET,
			echo.POST,
			echo.PUT,
			echo.PATCH,
			echo.DELETE,
			echo.HEAD,
			echo.OPTIONS,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"X-Session-Token",
			"X-CSRF-Token",
			"X-Requested-With",
		},
		ExposeHeaders: []string{
			"X-User-ID",
			"X-Tenant-ID",
			"X-Rate-Limit-Remaining",
			"X-Rate-Limit-Reset",
		},
		AllowCredentials: true,
		MaxAge:          86400, // 24 hours
	}
}

// NewCORSMiddleware creates a new CORS middleware with custom config
func NewCORSMiddleware(config CORSConfig) echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     config.AllowOrigins,
		AllowMethods:     config.AllowMethods,
		AllowHeaders:     config.AllowHeaders,
		ExposeHeaders:    config.ExposeHeaders,
		AllowCredentials: config.AllowCredentials,
		MaxAge:          config.MaxAge,
	})
}

// DefaultCORS returns CORS middleware with default configuration
func DefaultCORS() echo.MiddlewareFunc {
	return NewCORSMiddleware(DefaultCORSConfig())
}