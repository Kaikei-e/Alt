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

// ValidateParsedRSSURL performs security validation on a parsed RSS URL
func (v *URLSecurityValidator) ValidateParsedRSSURL(parsedURL *url.URL) error {
	if parsedURL == nil {
		return errors.New("nil URL")
	}

	// Validate scheme first (before checking host)
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("only HTTP and HTTPS schemes allowed")
	}

	if v.requireHTTPS && parsedURL.Scheme != "https" {
		return errors.New("HTTPS scheme is required for this endpoint")
	}

	// Reject userinfo
	if parsedURL.User != nil {
		return errors.New("userinfo not allowed in URL")
	}

	// Reject non-standard ports (allowed: absent, 80, 443; FEED_ALLOWED_HOSTS bypasses)
	port := parsedURL.Port()
	if port != "" && port != "80" && port != "443" && !IsFeedHostAllowed(parsedURL.Hostname()) {
		return errors.New("port not allowed")
	}

	// Check if URL has scheme and host (basic malformed URL detection)
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return errors.New("invalid URL format")
	}

	// Reject cloud metadata endpoints by exact hostname (M-004).
	hostname := strings.ToLower(parsedURL.Hostname())
	if IsMetadataHost(hostname) {
		return errors.New("metadata server access denied")
	}

	// Validate host for private networks
	if v.isPrivateNetwork(parsedURL.Hostname()) {
		return errors.New("private network access denied")
	}

	return nil
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

	return v.ValidateParsedRSSURL(parsedURL)
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
	if IsMetadataHost(domain) {
		return false
	}
	if v.isPrivateNetwork(domain) {
		return false
	}
	return true
}

// isPrivateNetwork checks if a hostname resolves to a private network
func (v *URLSecurityValidator) isPrivateNetwork(hostname string) bool {
	// An operator-allow-listed feed host (FEED_ALLOWED_HOSTS) is an explicit
	// trust decision and is never treated as a private-network threat — this is
	// what lets intentionally non-resolvable trusted hosts (e.g. the e2e stub)
	// through the SSRF gate. Strip any port before matching the allow-list.
	allowHost := hostname
	if h, _, err := net.SplitHostPort(hostname); err == nil {
		allowHost = h
	}
	if IsFeedHostAllowed(allowHost) {
		return false
	}

	// Check for localhost variants
	if hostname == "localhost" || hostname == "127.0.0.1" {
		return true
	}

	// Try to parse as IP address
	ip := net.ParseIP(hostname)
	if ip != nil {
		// Check private IP ranges using hardened IsPrivateIPAddress
		return IsPrivateIPAddress(ip)
	}

	// Fast-path check for common private domain suffixes before paying for
	// DNS resolution.
	if strings.HasSuffix(hostname, ".local") ||
		strings.HasSuffix(hostname, ".localhost") {
		return true
	}

	// SSRF finding [2]: a domain name must be resolved and every returned
	// address checked for private/loopback/link-local ranges — otherwise an
	// attacker-controlled domain whose A record points at, e.g.,
	// 169.254.169.254 (cloud metadata) sails through unresolved. Fail closed
	// (treat as private) when resolution fails, matching IsPrivateHost.
	return IsPrivateHost(hostname)
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
