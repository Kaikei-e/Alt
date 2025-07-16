package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// SecurityConfig holds security middleware configuration
type SecurityConfig struct {
	// Content Security Policy settings
	CSPDefaultSrc    []string
	CSPScriptSrc     []string
	CSPStyleSrc      []string
	CSPImgSrc        []string
	CSPConnectSrc    []string
	CSPFontSrc       []string
	CSPFrameAncestors []string
	
	// HSTS settings
	HSTSMaxAge            int
	HSTSIncludeSubdomains bool
	HSTSPreload           bool
	
	// Other security headers
	XFrameOptions        string
	XContentTypeOptions  string
	XSSProtection        string
	ReferrerPolicy       string
	PermissionsPolicy    string
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		// CSP settings
		CSPDefaultSrc:     []string{"'self'"},
		CSPScriptSrc:      []string{"'self'", "'unsafe-inline'"},
		CSPStyleSrc:       []string{"'self'", "'unsafe-inline'"},
		CSPImgSrc:         []string{"'self'", "data:", "https:"},
		CSPConnectSrc:     []string{"'self'", "wss:", "https:"},
		CSPFontSrc:        []string{"'self'", "https:"},
		CSPFrameAncestors: []string{"'none'"},
		
		// HSTS settings
		HSTSMaxAge:            31536000, // 1 year
		HSTSIncludeSubdomains: true,
		HSTSPreload:           false,
		
		// Other headers
		XFrameOptions:       "DENY",
		XContentTypeOptions: "nosniff",
		XSSProtection:       "1; mode=block",
		ReferrerPolicy:      "strict-origin-when-cross-origin",
		PermissionsPolicy:   "camera=(), microphone=(), geolocation=()",
	}
}

// SecurityMiddleware provides comprehensive security headers
type SecurityMiddleware struct {
	config *SecurityConfig
	logger *slog.Logger
}

// NewSecurityMiddleware creates a new security middleware
func NewSecurityMiddleware(config *SecurityConfig, logger *slog.Logger) *SecurityMiddleware {
	if config == nil {
		config = DefaultSecurityConfig()
	}
	
	return &SecurityMiddleware{
		config: config,
		logger: logger,
	}
}

// Middleware returns the security middleware function
func (m *SecurityMiddleware) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Set Content Security Policy
			csp := m.buildCSP()
			c.Response().Header().Set("Content-Security-Policy", csp)
			
			// Set HSTS header
			hsts := m.buildHSTS()
			c.Response().Header().Set("Strict-Transport-Security", hsts)
			
			// Set other security headers
			c.Response().Header().Set("X-Frame-Options", m.config.XFrameOptions)
			c.Response().Header().Set("X-Content-Type-Options", m.config.XContentTypeOptions)
			c.Response().Header().Set("X-XSS-Protection", m.config.XSSProtection)
			c.Response().Header().Set("Referrer-Policy", m.config.ReferrerPolicy)
			c.Response().Header().Set("Permissions-Policy", m.config.PermissionsPolicy)
			
			// Remove server identification
			c.Response().Header().Set("Server", "")
			
			// Prevent caching of sensitive pages
			if m.isSensitivePath(c.Path()) {
				c.Response().Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				c.Response().Header().Set("Pragma", "no-cache")
				c.Response().Header().Set("Expires", "0")
			}
			
			return next(c)
		}
	}
}

// buildCSP constructs the Content Security Policy header value
func (m *SecurityMiddleware) buildCSP() string {
	csp := fmt.Sprintf("default-src %s; ", joinSources(m.config.CSPDefaultSrc))
	csp += fmt.Sprintf("script-src %s; ", joinSources(m.config.CSPScriptSrc))
	csp += fmt.Sprintf("style-src %s; ", joinSources(m.config.CSPStyleSrc))
	csp += fmt.Sprintf("img-src %s; ", joinSources(m.config.CSPImgSrc))
	csp += fmt.Sprintf("connect-src %s; ", joinSources(m.config.CSPConnectSrc))
	csp += fmt.Sprintf("font-src %s; ", joinSources(m.config.CSPFontSrc))
	csp += fmt.Sprintf("frame-ancestors %s;", joinSources(m.config.CSPFrameAncestors))
	
	return csp
}

// buildHSTS constructs the HSTS header value
func (m *SecurityMiddleware) buildHSTS() string {
	hsts := fmt.Sprintf("max-age=%d", m.config.HSTSMaxAge)
	
	if m.config.HSTSIncludeSubdomains {
		hsts += "; includeSubDomains"
	}
	
	if m.config.HSTSPreload {
		hsts += "; preload"
	}
	
	return hsts
}

// isSensitivePath checks if the path should have no-cache headers
func (m *SecurityMiddleware) isSensitivePath(path string) bool {
	sensitivePaths := []string{
		"/v1/auth/",
		"/v1/user/",
		"/v1/admin/",
	}
	
	for _, sensitive := range sensitivePaths {
		if len(path) >= len(sensitive) && path[:len(sensitive)] == sensitive {
			return true
		}
	}
	
	return false
}

// joinSources joins CSP sources with spaces
func joinSources(sources []string) string {
	result := ""
	for i, source := range sources {
		if i > 0 {
			result += " "
		}
		result += source
	}
	return result
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Rate         float64       // Requests per second
	Burst        int           // Maximum burst size
	ExpiresIn    time.Duration // How long to remember IPs
	ErrorMessage string        // Custom error message
	
	// Different limits for different endpoint types
	AuthRate     float64 // Rate for auth endpoints
	UserRate     float64 // Rate for user endpoints
	DefaultRate  float64 // Default rate for other endpoints
}

// DefaultRateLimitConfig returns default rate limiting configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Rate:         20.0, // 20 requests per second
		Burst:        30,   // Allow burst of 30 requests
		ExpiresIn:    time.Hour,
		ErrorMessage: "Rate limit exceeded. Please try again later.",
		
		AuthRate:    5.0,  // Stricter limit for auth endpoints
		UserRate:    10.0, // Medium limit for user endpoints  
		DefaultRate: 20.0, // Default limit
	}
}

// RateLimitMiddleware provides rate limiting functionality
type RateLimitMiddleware struct {
	config *RateLimitConfig
	logger *slog.Logger
}

// NewRateLimitMiddleware creates a new rate limiting middleware
func NewRateLimitMiddleware(config *RateLimitConfig, logger *slog.Logger) *RateLimitMiddleware {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	
	return &RateLimitMiddleware{
		config: config,
		logger: logger,
	}
}

// Middleware returns the rate limiting middleware function
func (m *RateLimitMiddleware) Middleware() echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStore(rate.Limit(m.config.Rate)),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			// Rate limit by IP + User ID if available
			ip := c.RealIP()
			if userID := c.Get("user_id"); userID != nil {
				return fmt.Sprintf("%s:%s", ip, userID), nil
			}
			return ip, nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			m.logger.Warn("rate limit exceeded", 
				"ip", c.RealIP(),
				"path", c.Path(),
				"method", c.Request().Method,
				"error", err)
			
			// Set rate limit headers
			c.Response().Header().Set("X-Rate-Limit-Limit", fmt.Sprintf("%.0f", m.config.Rate))
			c.Response().Header().Set("X-Rate-Limit-Remaining", "0")
			c.Response().Header().Set("X-Rate-Limit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Minute).Unix()))
			
			return c.JSON(http.StatusTooManyRequests, map[string]string{
				"error": m.config.ErrorMessage,
			})
		},
		Skipper: func(c echo.Context) bool {
			// Skip rate limiting for health checks
			return c.Path() == "/v1/health" || c.Path() == "/v1/ready" || c.Path() == "/v1/live"
		},
	})
}

// AuthRateLimit returns rate limiting middleware specifically for auth endpoints
func (m *RateLimitMiddleware) AuthRateLimit() echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStore(rate.Limit(m.config.AuthRate)),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			// For auth endpoints, rate limit by IP only (no user context yet)
			return c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			m.logger.Warn("auth rate limit exceeded", 
				"ip", c.RealIP(),
				"path", c.Path(),
				"method", c.Request().Method)
			
			return c.JSON(http.StatusTooManyRequests, map[string]string{
				"error": "Too many authentication attempts. Please try again later.",
			})
		},
	})
}

// UserRateLimit returns rate limiting middleware for user endpoints
func (m *RateLimitMiddleware) UserRateLimit() echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStore(rate.Limit(m.config.UserRate)),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			// Rate limit by User ID for authenticated endpoints
			if userID := c.Get("user_id"); userID != nil {
				return userID.(string), nil
			}
			// Fallback to IP if no user context
			return c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			m.logger.Warn("user rate limit exceeded", 
				"ip", c.RealIP(),
				"user_id", c.Get("user_id"),
				"path", c.Path(),
				"method", c.Request().Method)
			
			return c.JSON(http.StatusTooManyRequests, map[string]string{
				"error": "Too many requests. Please slow down.",
			})
		},
	})
}

// RequestLoggingMiddleware provides detailed request logging for security monitoring
func RequestLoggingMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			
			// Process request
			err := next(c)
			
			// Log security-relevant information after processing
			userID := c.Get("user_id")
			sessionID := c.Get("session_id")
			
			logData := map[string]interface{}{
				"method":     c.Request().Method,
				"uri":        c.Request().RequestURI,
				"ip":         c.RealIP(),
				"user_agent": c.Request().UserAgent(),
				"status":     c.Response().Status,
				"size":       c.Response().Size,
				"duration":   time.Since(start).Milliseconds(),
			}
			
			if userID != nil {
				logData["user_id"] = userID
			}
			
			if sessionID != nil {
				logData["session_id"] = sessionID
			}
			
			// Log suspicious patterns
			if isSuspiciousRequest(c) {
				logData["suspicious"] = true
				logger.Warn("suspicious request detected", 
					"ip", c.RealIP(),
					"path", c.Path(),
					"method", c.Request().Method,
					"user_agent", c.Request().UserAgent())
			}
			
			logger.Info("request processed", 
				"method", logData["method"],
				"uri", logData["uri"],
				"ip", logData["ip"],
				"status", logData["status"],
				"user_id", logData["user_id"],
				"duration_ms", logData["duration"])
			
			return err
		}
	}
}

// isSuspiciousRequest checks for suspicious request patterns
func isSuspiciousRequest(c echo.Context) bool {
	userAgent := c.Request().UserAgent()
	path := c.Path()
	
	// Check for common attack patterns
	suspiciousUserAgents := []string{
		"sqlmap",
		"nikto",
		"nmap",
		"masscan",
		"burp",
		"dirb",
		"gobuster",
	}
	
	for _, suspicious := range suspiciousUserAgents {
		if len(userAgent) >= len(suspicious) && 
		   userAgent[:min(len(userAgent), len(suspicious))] == suspicious {
			return true
		}
	}
	
	// Check for path traversal attempts
	if containsPathTraversal(path) {
		return true
	}
	
	// Check for SQL injection patterns in query params
	if containsSQLInjection(c.QueryString()) {
		return true
	}
	
	return false
}

// containsPathTraversal checks for path traversal patterns
func containsPathTraversal(path string) bool {
	patterns := []string{"../", "..\\", "%2e%2e%2f", "%2e%2e%5c"}
	
	for _, pattern := range patterns {
		if len(path) >= len(pattern) {
			for i := 0; i <= len(path)-len(pattern); i++ {
				if path[i:i+len(pattern)] == pattern {
					return true
				}
			}
		}
	}
	
	return false
}

// containsSQLInjection checks for basic SQL injection patterns
func containsSQLInjection(query string) bool {
	patterns := []string{
		"' or '1'='1",
		"' or 1=1--",
		"union select",
		"drop table",
		"insert into",
		"delete from",
	}
	
	queryLower := toLower(query)
	
	for _, pattern := range patterns {
		if len(queryLower) >= len(pattern) {
			for i := 0; i <= len(queryLower)-len(pattern); i++ {
				if queryLower[i:i+len(pattern)] == pattern {
					return true
				}
			}
		}
	}
	
	return false
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			result[i] = s[i] + 32
		} else {
			result[i] = s[i]
		}
	}
	return string(result)
}