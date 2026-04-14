package security

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// URLSecurityValidator provides comprehensive URL security validation functionality
// for RSS feed registration endpoints. It implements multiple layers of security
// validation including scheme validation, private network detection, and
// malicious URL pattern detection.
type URLSecurityValidator struct {
	// requireHTTPS, when true, rejects http:// URLs. Use for callers like the
	// image proxy and RAG fetcher where plaintext HTTP is unacceptable (M-002).
	requireHTTPS bool
}

// NewURLSecurityValidator creates a new URLSecurityValidator instance.
func NewURLSecurityValidator() *URLSecurityValidator {
	return &URLSecurityValidator{}
}

// RequireHTTPS toggles HTTPS-only mode. Default is false (HTTP allowed for
// RSS feed registration which still uses many plaintext endpoints).
func (v *URLSecurityValidator) RequireHTTPS(require bool) {
	v.requireHTTPS = require
}

// metadataHosts enumerates known cloud metadata hostnames. Exact-match lookup
// (M-004) avoids the substring false positives of strings.Contains.
var metadataHosts = map[string]struct{}{
	"169.254.169.254":          {},
	"metadata.google.internal": {},
	"100.100.100.200":          {},
	"192.0.0.192":              {},
}

// ValidateRSSURL performs comprehensive security validation on RSS URLs
func (v *URLSecurityValidator) ValidateRSSURL(rawURL string) error {
	// Check for empty URL
	if rawURL == "" {
		return errors.New("URL cannot be empty")
	}

	// Check URL length to prevent extremely long URLs
	if len(rawURL) > 2048 {
		return errors.New("URL exceeds maximum length")
	}

	// Check for dangerous patterns
	if strings.Contains(rawURL, "..") {
		return errors.New("URL contains dangerous pattern")
	}

	// Parse URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid URL format")
	}

	// Validate scheme first (before checking host)
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("only HTTP and HTTPS schemes allowed")
	}

	if v.requireHTTPS && parsedURL.Scheme != "https" {
		return errors.New("HTTPS scheme is required for this endpoint")
	}

	// Check if URL has scheme and host (basic malformed URL detection)
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return errors.New("invalid URL format")
	}

	// Reject cloud metadata endpoints by exact hostname (M-004).
	hostname := strings.ToLower(parsedURL.Hostname())
	if _, isMetadata := metadataHosts[hostname]; isMetadata {
		return errors.New("metadata server access denied")
	}

	// Validate host for private networks
	if v.isPrivateNetwork(parsedURL.Host) {
		return errors.New("private network access denied")
	}

	return nil
}

// ValidateForRSSFeed performs RSS-specific validation
func (v *URLSecurityValidator) ValidateForRSSFeed(rawURL string) error {
	// First perform basic URL validation
	if err := v.ValidateRSSURL(rawURL); err != nil {
		return err
	}

	// Parse URL for RSS-specific checks
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid URL format")
	}

	// Check if path appears to be RSS-related
	if !v.isValidRSSPath(parsedURL.Path) {
		return errors.New("URL path does not appear to be an RSS feed")
	}

	return nil
}

// IsAllowedDomain checks if a domain is allowed for RSS feed access.
// Metadata endpoints are rejected by exact hostname match (M-004).
func (v *URLSecurityValidator) IsAllowedDomain(domain string) bool {
	if domain == "localhost" {
		return false
	}
	if _, isMetadata := metadataHosts[strings.ToLower(domain)]; isMetadata {
		return false
	}
	if v.isPrivateNetwork(domain) {
		return false
	}
	return true
}

// isPrivateNetwork checks if a hostname resolves to a private network
func (v *URLSecurityValidator) isPrivateNetwork(hostname string) bool {
	// Check for localhost variants
	if hostname == "localhost" || hostname == "127.0.0.1" {
		return true
	}

	// Try to parse as IP address
	ip := net.ParseIP(hostname)
	if ip != nil {
		// Check private IP ranges
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}

	// For domain names, we cannot easily check without DNS resolution
	// but we can check for common private domain patterns
	if strings.HasSuffix(hostname, ".local") ||
		strings.HasSuffix(hostname, ".localhost") {
		return true
	}

	return false
}

// isValidRSSPath checks if the URL path appears to be RSS-related
func (v *URLSecurityValidator) isValidRSSPath(path string) bool {
	path = strings.ToLower(path)

	// Common RSS/Atom/Feed patterns
	validPatterns := []string{
		"rss", "feed", "atom", "xml", "feeds",
	}

	for _, pattern := range validPatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}

	return false
}
