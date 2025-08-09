package proxy

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// extractTargetURL extracts and validates the target URL from the proxy request path
// This function is central to the upstream resolution fix described in ISSUE_RESOLVE_PLAN.md
func (p *LightweightProxy) extractTargetURL(requestPath string) (*url.URL, error) {
	// Step 1: Extract URL from path like "/proxy/https://zenn.dev/feed"
	const proxyPrefix = "/proxy/"
	if !strings.HasPrefix(requestPath, proxyPrefix) {
		return nil, fmt.Errorf("invalid proxy path: %s (expected /proxy/...)", requestPath)
	}

	urlStr := strings.TrimPrefix(requestPath, proxyPrefix)
	if urlStr == "" {
		return nil, fmt.Errorf("empty target URL after removing proxy prefix")
	}

	p.logger.Printf("Raw URL extracted: %s", urlStr)

	// Step 2: CVE-2024-34155 mitigation - check URL parsing complexity before parsing
	if err := p.checkParsingComplexity(urlStr); err != nil {
		return nil, fmt.Errorf("URL parsing complexity check failed: %w", err)
	}

	// Step 3: Sanitize URL string to handle common malformed URL issues
	sanitizedURL := p.sanitizeURLString(urlStr)
	p.logger.Printf("Sanitized URL: %s", sanitizedURL)

	// Step 4: Parse the URL safely
	targetURL, err := url.Parse(sanitizedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target URL '%s': %w", sanitizedURL, err)
	}

	// Step 5: Basic URL validation
	if targetURL.Scheme == "" {
		return nil, fmt.Errorf("missing URL scheme")
	}
	if targetURL.Host == "" {
		return nil, fmt.Errorf("missing URL host")
	}

	// Step 6: Keep HTTPS URLs without explicit port 443 (standard practice)
	// Most web servers prefer https://example.com over https://example.com:443
	// Explicit default ports can cause 403/400 errors on some servers

	// Step 7: Validate URL components using allowlist approach (2024 best practice)
	if err := p.validateURLSecurity(targetURL); err != nil {
		return nil, fmt.Errorf("URL security validation failed: %w", err)
	}

	return targetURL, nil
}

// checkParsingComplexity validates URL complexity to prevent CVE-2024-34155 stack exhaustion
// Implements parsing depth limitations as recommended by Go security team
func (p *LightweightProxy) checkParsingComplexity(urlStr string) error {
	// Check for excessive nesting patterns that could trigger stack exhaustion
	const maxNestedPercent = 50      // Maximum percentage of URL that can be percent-encoded
	const maxConsecutiveSlashes = 10 // Maximum consecutive slashes
	const maxConsecutivePercent = 20 // Maximum consecutive percent signs

	// Count percent-encoded characters (CVE-2024-34155 protection)
	percentCount := strings.Count(urlStr, "%")
	if percentCount > len(urlStr)*maxNestedPercent/100 {
		return fmt.Errorf("excessive URL encoding detected: %d percent signs in %d characters", percentCount, len(urlStr))
	}

	// Check for consecutive slashes that could cause parsing issues
	consecutiveSlashes := 0
	maxConsecutiveSlashesFound := 0
	for _, char := range urlStr {
		if char == '/' {
			consecutiveSlashes++
			if consecutiveSlashes > maxConsecutiveSlashesFound {
				maxConsecutiveSlashesFound = consecutiveSlashes
			}
		} else {
			consecutiveSlashes = 0
		}
	}
	if maxConsecutiveSlashesFound > maxConsecutiveSlashes {
		return fmt.Errorf("excessive consecutive slashes detected: %d", maxConsecutiveSlashesFound)
	}

	// Check for consecutive percent signs
	consecutivePercent := 0
	for i := 0; i < len(urlStr); i++ {
		if urlStr[i] == '%' {
			consecutivePercent++
			if consecutivePercent > maxConsecutivePercent {
				return fmt.Errorf("excessive consecutive percent signs detected: %d", consecutivePercent)
			}
		} else {
			consecutivePercent = 0
		}
	}

	return nil
}

// sanitizeURLString performs safe URL string cleanup without regex
// Addresses common URL format issues using string operations only (2024 security best practice)
func (p *LightweightProxy) sanitizeURLString(urlStr string) string {
	// Remove dangerous whitespace characters
	urlStr = strings.TrimSpace(urlStr)

	// Fix single slash issues (https:/ -> https://) using safe string operations
	if strings.HasPrefix(urlStr, "https:/") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + strings.TrimPrefix(urlStr, "https:/")
	}
	if strings.HasPrefix(urlStr, "http:/") && !strings.HasPrefix(urlStr, "http://") {
		urlStr = "http://" + strings.TrimPrefix(urlStr, "http:/")
	}

	// Remove control characters that could be used for bypass attempts
	// Using strings.Map for safe character filtering without regex
	urlStr = strings.Map(func(r rune) rune {
		// Allow only printable ASCII characters and common URL characters
		if r >= 32 && r <= 126 {
			return r
		}
		// Remove control characters and non-ASCII characters
		return -1
	}, urlStr)

	return urlStr
}

// validateURLSecurity implements comprehensive URL security validation
// Following OWASP SSRF Prevention guidelines and 2024 CVE mitigations
func (p *LightweightProxy) validateURLSecurity(targetURL *url.URL) error {
	// Security Check 1: Scheme validation (only HTTPS allowed for RSS feeds)
	if targetURL.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed for security, got: %s", targetURL.Scheme)
	}

	// Security Check 2: Host validation with enhanced checks
	if targetURL.Host == "" {
		return fmt.Errorf("missing host in URL")
	}

	// Security Check 3: Enhanced host validation for 2024 security threats
	if err := p.validateHostSecurity(targetURL.Host); err != nil {
		return fmt.Errorf("host security validation failed: %w", err)
	}

	// Security Check 4: Domain allowlist validation (OWASP 2024 best practice)
	// Extract hostname without port for allowlist check
	hostname := targetURL.Hostname()
	if !p.config.IsDomainAllowed(hostname) {
		return fmt.Errorf("domain not in security allowlist: %s", hostname)
	}

	// Security Check 5: Enhanced path validation
	if err := p.validatePathSecurity(targetURL.Path); err != nil {
		return fmt.Errorf("path security validation failed: %w", err)
	}

	// Security Check 6: Query parameter validation to prevent injection
	if err := p.validateQuerySecurity(targetURL.RawQuery); err != nil {
		return fmt.Errorf("query parameter security validation failed: %w", err)
	}

	// Security Check 7: Fragment validation (XSS prevention)
	if err := p.validateFragmentSecurity(targetURL.Fragment); err != nil {
		return fmt.Errorf("fragment security validation failed: %w", err)
	}

	// Security Check 8: Port validation (prevent bypass attempts)
	if err := p.validatePortSecurity(targetURL.Port()); err != nil {
		return fmt.Errorf("port security validation failed: %w", err)
	}

	return nil
}

// validateHostSecurity prevents SSRF attacks by validating host security
// Implements 2024 best practices for preventing access to internal resources
func (p *LightweightProxy) validateHostSecurity(host string) error {
	// Remove port from host for validation
	hostname := host
	if strings.Contains(host, ":") {
		var err error
		hostname, _, err = net.SplitHostPort(host)
		if err != nil {
			return fmt.Errorf("invalid host format: %w", err)
		}
	}

	// Check for localhost variants
	localhostVariants := []string{
		"localhost", "127.0.0.1", "::1", "0.0.0.0",
		"0", "0x0", "0x00000000", "2130706433", // IPv4 localhost representations
	}
	for _, variant := range localhostVariants {
		if strings.EqualFold(hostname, variant) {
			return fmt.Errorf("localhost access denied for security")
		}
	}

	// Check for private network ranges (RFC 1918)
	if ip := net.ParseIP(hostname); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			return fmt.Errorf("access to private/loopback IP denied: %s", hostname)
		}
	}

	// Additional check for metadata service endpoints (cloud security)
	metadataEndpoints := []string{
		"169.254.169.254", // AWS/GCP metadata
		"metadata.google.internal",
		"169.254.169.254:80",
	}
	for _, endpoint := range metadataEndpoints {
		if strings.EqualFold(hostname, endpoint) {
			return fmt.Errorf("metadata service access denied")
		}
	}

	return nil
}

// validatePathSecurity performs enhanced path validation (2024 security practices)
func (p *LightweightProxy) validatePathSecurity(path string) error {
	// Directory traversal prevention with multiple encoding forms
	dangerousPatterns := []string{
		"..", "/..", "../", "%2e%2e", "%2E%2E", // Basic directory traversal
		"%252e%252e", "%c0%ae", "%c1%9c", // Double-encoded and unicode variants
		"\\", "%5c", "%255c", // Backslash variants (Windows)
	}

	pathLower := strings.ToLower(path)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(pathLower, pattern) {
			return fmt.Errorf("dangerous path pattern detected: %s", pattern)
		}
	}

	// Path length validation
	if len(path) > 512 {
		return fmt.Errorf("path too long: %d characters (max 512)", len(path))
	}

	return nil
}

// validateQuerySecurity validates query parameters for security threats
func (p *LightweightProxy) validateQuerySecurity(rawQuery string) error {
	if rawQuery == "" {
		return nil // Empty queries are safe
	}

	// Query length validation
	if len(rawQuery) > 1024 {
		return fmt.Errorf("query string too long: %d characters (max 1024)", len(rawQuery))
	}

	// Check for SQL injection patterns
	sqlPatterns := []string{
		"union", "select", "insert", "delete", "update", "drop", "exec", "script",
		"'", "\"", ";", "--", "/*", "*/", "xp_", "sp_",
	}

	queryLower := strings.ToLower(rawQuery)
	for _, pattern := range sqlPatterns {
		if strings.Contains(queryLower, pattern) {
			return fmt.Errorf("potentially dangerous query pattern detected: %s", pattern)
		}
	}

	return nil
}

// validateFragmentSecurity validates URL fragments for XSS prevention
func (p *LightweightProxy) validateFragmentSecurity(fragment string) error {
	if fragment == "" {
		return nil // Empty fragments are safe
	}

	// Fragment length validation
	if len(fragment) > 256 {
		return fmt.Errorf("fragment too long: %d characters (max 256)", len(fragment))
	}

	// XSS prevention - check for dangerous JavaScript patterns
	dangerousFragments := []string{
		"javascript:", "data:", "vbscript:", "onload", "onerror", "onclick",
		"<script", "</script>", "eval(", "alert(", "document.", "window.",
	}

	fragmentLower := strings.ToLower(fragment)
	for _, pattern := range dangerousFragments {
		if strings.Contains(fragmentLower, pattern) {
			return fmt.Errorf("dangerous fragment pattern detected: %s", pattern)
		}
	}

	return nil
}

// validatePortSecurity validates URL ports for security bypass prevention
func (p *LightweightProxy) validatePortSecurity(port string) error {
	if port == "" {
		return nil // Default HTTPS port (443) is allowed
	}

	// Only allow standard HTTPS port
	if port != "443" {
		return fmt.Errorf("only standard HTTPS port (443) is allowed, got: %s", port)
	}

	return nil
}