package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
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
	config   *RateLimitConfig
	logger   *slog.Logger
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
}

// NewRateLimitMiddleware creates a new rate limiting middleware
func NewRateLimitMiddleware(config *RateLimitConfig, logger *slog.Logger) *RateLimitMiddleware {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	
	return &RateLimitMiddleware{
		config:   config,
		logger:   logger,
		limiters: make(map[string]*rate.Limiter),
		mu:       sync.RWMutex{},
	}
}

// Middleware returns the rate limiting middleware function
func (m *RateLimitMiddleware) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip rate limiting for health checks
			if c.Path() == "/v1/health" || c.Path() == "/v1/ready" || c.Path() == "/v1/live" {
				return next(c)
			}
			
			// Get identifier for rate limiting
			identifier := c.RealIP()
			if userID := c.Get("user_id"); userID != nil {
				identifier = fmt.Sprintf("%s:%s", identifier, userID)
			}
			
			// Get limiter for this identifier
			limiter := m.getLimiter(identifier)
			
			// Check if request is allowed
			if !limiter.Allow() {
				m.logger.Warn("rate limit exceeded", 
					"ip", c.RealIP(),
					"path", c.Path(),
					"method", c.Request().Method,
					"identifier", identifier)
				
				// Set rate limit headers
				c.Response().Header().Set("X-Rate-Limit-Limit", fmt.Sprintf("%.0f", m.config.Rate))
				c.Response().Header().Set("X-Rate-Limit-Remaining", "0")
				c.Response().Header().Set("X-Rate-Limit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Minute).Unix()))
				
				return c.JSON(http.StatusTooManyRequests, map[string]string{
					"error": m.config.ErrorMessage,
				})
			}
			
			return next(c)
		}
	}
}

// getLimiter returns a rate limiter for the given identifier
func (m *RateLimitMiddleware) getLimiter(identifier string) *rate.Limiter {
	m.mu.RLock()
	limiter, exists := m.limiters[identifier]
	m.mu.RUnlock()
	
	if !exists {
		m.mu.Lock()
		// Double-check after acquiring write lock
		limiter, exists = m.limiters[identifier]
		if !exists {
			limiter = rate.NewLimiter(rate.Limit(m.config.Rate), m.config.Burst)
			m.limiters[identifier] = limiter
		}
		m.mu.Unlock()
	}
	
	return limiter
}

// getAuthLimiter returns a rate limiter for auth endpoints
func (m *RateLimitMiddleware) getAuthLimiter(identifier string) *rate.Limiter {
	authKey := "auth:" + identifier
	m.mu.RLock()
	limiter, exists := m.limiters[authKey]
	m.mu.RUnlock()
	
	if !exists {
		m.mu.Lock()
		limiter, exists = m.limiters[authKey]
		if !exists {
			limiter = rate.NewLimiter(rate.Limit(m.config.AuthRate), m.config.Burst)
			m.limiters[authKey] = limiter
		}
		m.mu.Unlock()
	}
	
	return limiter
}

// getUserLimiter returns a rate limiter for user endpoints
func (m *RateLimitMiddleware) getUserLimiter(identifier string) *rate.Limiter {
	userKey := "user:" + identifier
	m.mu.RLock()
	limiter, exists := m.limiters[userKey]
	m.mu.RUnlock()
	
	if !exists {
		m.mu.Lock()
		limiter, exists = m.limiters[userKey]
		if !exists {
			limiter = rate.NewLimiter(rate.Limit(m.config.UserRate), m.config.Burst)
			m.limiters[userKey] = limiter
		}
		m.mu.Unlock()
	}
	
	return limiter
}

// AuthRateLimit returns rate limiting middleware specifically for auth endpoints
func (m *RateLimitMiddleware) AuthRateLimit() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// For auth endpoints, rate limit by IP only (no user context yet)
			identifier := c.RealIP()
			limiter := m.getAuthLimiter(identifier)
			
			if !limiter.Allow() {
				m.logger.Warn("auth rate limit exceeded", 
					"ip", c.RealIP(),
					"path", c.Path(),
					"method", c.Request().Method)
				
				return c.JSON(http.StatusTooManyRequests, map[string]string{
					"error": "Too many authentication attempts. Please try again later.",
				})
			}
			
			return next(c)
		}
	}
}

// UserRateLimit returns rate limiting middleware for user endpoints
func (m *RateLimitMiddleware) UserRateLimit() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Rate limit by User ID for authenticated endpoints
			identifier := c.RealIP()
			if userID := c.Get("user_id"); userID != nil {
				identifier = userID.(string)
			}
			
			limiter := m.getUserLimiter(identifier)
			
			if !limiter.Allow() {
				m.logger.Warn("user rate limit exceeded", 
					"ip", c.RealIP(),
					"user_id", c.Get("user_id"),
					"path", c.Path(),
					"method", c.Request().Method)
				
				return c.JSON(http.StatusTooManyRequests, map[string]string{
					"error": "Too many requests. Please slow down.",
				})
			}
			
			return next(c)
		}
	}
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
	path := c.Request().URL.Path
	
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
		if containsString(toLower(userAgent), suspicious) {
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
		if containsString(path, pattern) {
			return true
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

func containsString(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}