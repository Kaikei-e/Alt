package rest

import (
	"alt/domain"
	"alt/utils/errors"
	"alt/utils/logger"
	stderrors "errors"
	"net"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
)

// handleError converts errors to appropriate HTTP responses using enhanced error handling
func handleError(c echo.Context, err error, operation string) error {
	// Enrich error with REST layer context
	var enrichedErr *errors.AppContextError

	// Check if it's already an AppContextError and enrich it with REST context
	if appContextErr, ok := err.(*errors.AppContextError); ok {
		enrichedErr = errors.EnrichWithContext(
			appContextErr,
			"rest",
			"RESTHandler",
			operation,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	} else if appErr, ok := err.(*errors.AppError); ok {
		// Handle legacy AppError by converting to AppContextError
		enrichedErr = errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			"rest",
			"RESTHandler",
			operation,
			appErr.Cause,
			map[string]interface{}{
				"path":           c.Request().URL.Path,
				"method":         c.Request().Method,
				"remote_addr":    c.Request().RemoteAddr,
				"user_agent":     c.Request().UserAgent(),
				"request_id":     c.Response().Header().Get("X-Request-ID"),
				"legacy_context": appErr.Context,
			},
		)
	} else {
		// Handle unknown errors
		enrichedErr = errors.NewUnknownContextError(
			"internal server error",
			"rest",
			"RESTHandler",
			operation,
			err,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	}

	// Log the enriched error with context
	logger.Logger.Error("REST handler error",
		"error", enrichedErr.Error(),
		"error_code", enrichedErr.Code,
		"layer", enrichedErr.Layer,
		"component", enrichedErr.Component,
		"operation", enrichedErr.Operation,
		"path", c.Request().URL.Path,
		"method", c.Request().Method,
		"is_retryable", enrichedErr.IsRetryable(),
	)

	return c.JSON(enrichedErr.HTTPStatusCode(), enrichedErr.ToHTTPResponse())
}

// handleValidationError creates a validation error response with enhanced context
func handleValidationError(c echo.Context, message string, field string, value interface{}) error {
	validationErr := errors.NewValidationContextError(
		message,
		"rest",
		"RESTHandler",
		"validateInput",
		map[string]interface{}{
			"field":       field,
			"value":       value,
			"path":        c.Request().URL.Path,
			"method":      c.Request().Method,
			"remote_addr": c.Request().RemoteAddr,
			"user_agent":  c.Request().UserAgent(),
			"request_id":  c.Response().Header().Get("X-Request-ID"),
		},
	)

	logger.Logger.Error("REST validation error",
		"error", validationErr.Error(),
		"field", field,
		"value", value,
		"path", c.Request().URL.Path,
	)
	return c.JSON(validationErr.HTTPStatusCode(), validationErr.ToHTTPResponse())
}

func isAllowedURL(u *url.URL) error {
	// Allow both HTTP and HTTPS
	if u.Scheme != "https" && u.Scheme != "http" {
		return stderrors.New("only HTTP and HTTPS schemes allowed")
	}

	// Block private networks
	if isPrivateIP(u.Hostname()) {
		return stderrors.New("access to private networks not allowed")
	}

	// Block localhost variations
	hostname := strings.ToLower(u.Hostname())
	if hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.") {
		return stderrors.New("access to localhost not allowed")
	}

	// Block metadata endpoints (AWS, GCP, Azure)
	if hostname == "169.254.169.254" || hostname == "metadata.google.internal" {
		return stderrors.New("access to metadata endpoint not allowed")
	}

	// Block common internal domains
	internalDomains := []string{".local", ".internal", ".corp", ".lan"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostname, domain) {
			return stderrors.New("access to internal domains not allowed")
		}
	}

	return nil
}

func isPrivateIP(hostname string) bool {
	// Try to parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		return isPrivateIPAddress(ip)
	}

	// If it's a hostname, resolve it to IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Block on resolution failure as a security measure
		return true
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIPAddress(ip) {
			return true
		}
	}

	return false
}

func isPrivateIPAddress(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	if ip.To4() != nil {
		// 10.0.0.0/8
		if ip[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip[0] == 192 && ip[1] == 168 {
			return true
		}
	}

	// Check for private IPv6 ranges
	if ip.To16() != nil && ip.To4() == nil {
		// Check for unique local addresses (fc00::/7)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return true
		}
	}

	return false
}

// Optimize feeds response by truncating descriptions and removing unnecessary fields
func optimizeFeedsResponse(feeds []*domain.FeedItem) []*domain.FeedItem {
	for _, feed := range feeds {
		feed.Title = strings.TrimSpace(feed.Title)
		// Simplified optimization for now
		if len(feed.Description) > 500 {
			feed.Description = feed.Description[:500] + "..."
		}
	}
	return feeds
}

// Determine cache age based on limit to optimize caching strategy
func getCacheAgeForLimit(limit int) int {
	switch {
	case limit <= 20:
		return 600 // 10 minutes for small requests
	case limit <= 100:
		return 900 // 15 minutes for medium requests
	default:
		return 1800 // 30 minutes for large requests
	}
}