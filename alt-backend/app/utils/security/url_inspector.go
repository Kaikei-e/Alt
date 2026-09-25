package security

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
	"golang.org/x/text/unicode/norm"
)

// metadataEndpointsList lists known cloud provider instance metadata endpoints.
var metadataEndpointsList = []string{
	"169.254.169.254",          // AWS, Azure, and GCP metadata IP
	"metadata.google.internal", // GCP metadata hostname
	"100.100.100.200",          // Alibaba Cloud metadata IP
	"192.0.0.192",              // Oracle Cloud metadata IP
	"169.254.169.254:80",       // Explicit port AWS metadata endpoint
	"169.254.169.254:8080",     // Explicit alternative port AWS metadata endpoint
}

// metadataHosts enumerates known cloud metadata hostnames. Exact-match lookup
// (M-004) avoids the substring false positives of strings.Contains.
var metadataHosts = func() map[string]struct{} {
	m := make(map[string]struct{}, len(metadataEndpointsList))
	for _, ep := range metadataEndpointsList {
		m[ep] = struct{}{}
	}
	return m
}()

// defaultInternalDomains contains domain suffixes reserved for private networks.
var defaultInternalDomains = []string{
	".local", ".internal", ".corp", ".lan", ".intranet",
	".test", ".localhost", ".cluster.local",
}

// canonicalRequestURLRe is the shape CanonicalRequestURL may return: http(s),
// no userinfo, non-empty host. Unexported so it is not an SSRF-safety API.
var canonicalRequestURLRe = regexp.MustCompile(`^https?://[^/?#@]+(?:[/?#].*)?$`)

// IsMetadataHost reports whether hostname matches a known cloud metadata endpoint.
func IsMetadataHost(hostname string) bool {
	h := strings.ToLower(strings.TrimSpace(hostname))
	if _, ok := metadataHosts[h]; ok {
		return true
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		if _, ok := metadataHosts[host]; ok {
			return true
		}
	}
	return false
}

// hasInternalDomainSuffix checks if a hostname ends with any restricted internal domain suffix.
func hasInternalDomainSuffix(hostname string, suffixes []string) bool {
	h := strings.ToLower(hostname)
	for _, suffix := range suffixes {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

// allowlistedRequestScheme verifies that scheme is limited to http or https.
func allowlistedRequestScheme(scheme string) (string, error) {
	switch strings.ToLower(scheme) {
	case "https":
		return "https", nil
	case "http":
		return "http", nil
	default:
		return "", &ValidationError{
			Message: "only HTTP and HTTPS schemes allowed",
			Type:    "SCHEME_VALIDATION_ERROR",
		}
	}
}

// validatePathSyntax inspects paths for traversal attempts and forbidden control character encodings.
func validatePathSyntax(path, rawPath string) error {
	pathToCheck := path
	if rawPath != "" {
		pathToCheck = rawPath
	}

	// Only block control characters and backslash encoding in paths.
	// Encoded dots (%2e) and slashes (%2f) are NOT blocked here because:
	//   1. CDN URLs (dev.to, Cloudinary, Imgix) legitimately use %2F/%2C in paths
	//   2. Path traversal via %2e%2e is caught by the decoded-path ".." check below
	//   3. Go's url.Parse decodes %2e%2e → ".." which the next check detects
	suspiciousPatterns := []string{
		"%00",        // null byte
		"%0a", "%0A", // newline
		"%0d", "%0D", // carriage return
		"%5c", "%5C", // encoded backslash
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(pathToCheck, pattern) {
			return &ValidationError{
				Message: "URL encoding attacks not allowed in path",
				Type:    "URL_ENCODING_BLOCKED",
			}
		}
	}

	// Check for path traversal patterns (after URL decoding)
	if strings.Contains(path, "..") || strings.Contains(path, "/.") {
		return &ValidationError{
			Message: "path traversal patterns not allowed",
			Type:    "PATH_TRAVERSAL_BLOCKED",
		}
	}

	return nil
}

// validatePortNumber checks whether the port is permitted under policy.
func validatePortNumber(port string, allowedPorts map[string]bool, allowTestingLocalhost bool, hostname string) error {
	if allowTestingLocalhost {
		if hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.") {
			return nil
		}
	}

	if port != "" && !allowedPorts[port] {
		return &ValidationError{
			Message: fmt.Sprintf("non-standard port not allowed: %s", port),
			Type:    "PORT_BLOCKED",
			Details: map[string]interface{}{
				"port": port,
			},
		}
	}
	return nil
}

// validateUnicodeAndPunycode inspects hostnames for IDNA punycode bypasses and homograph confusables.
func validateUnicodeAndPunycode(hostname string, allowTestingLocalhost bool) error {
	isTestingLocalhost := allowTestingLocalhost &&
		(hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127."))

	if isTestingLocalhost {
		if hasMixedScripts(hostname) {
			return &ValidationError{
				Message: "mixed script attack detected",
				Type:    "MIXED_SCRIPT_BLOCKED",
				Details: map[string]interface{}{"hostname": hostname},
			}
		}
		if hasConfusableChars(hostname) {
			return &ValidationError{
				Message: "unicode bypass detected",
				Type:    "UNICODE_BYPASS_BLOCKED",
				Details: map[string]interface{}{"hostname": hostname},
			}
		}
		return nil
	}

	asciiHostname, err := idna.ToASCII(hostname)
	if err != nil {
		return &ValidationError{
			Message: "invalid internationalized domain name",
			Type:    "PUNYCODE_VALIDATION_ERROR",
		}
	}

	if hostname != asciiHostname {
		if strings.Contains(asciiHostname, "localhost") ||
			strings.Contains(asciiHostname, "127.") ||
			strings.Contains(asciiHostname, "10.") ||
			strings.Contains(asciiHostname, "192.168.") ||
			strings.Contains(asciiHostname, "172.") {
			return &ValidationError{
				Message: "punycode bypass detected",
				Type:    "PUNYCODE_BYPASS_BLOCKED",
				Details: map[string]interface{}{
					"original": hostname,
					"ascii":    asciiHostname,
				},
			}
		}
	}

	if hasMixedScripts(hostname) {
		return &ValidationError{
			Message: "mixed script attack detected",
			Type:    "MIXED_SCRIPT_BLOCKED",
			Details: map[string]interface{}{"hostname": hostname},
		}
	}

	if hasConfusableChars(hostname) {
		return &ValidationError{
			Message: "unicode bypass detected",
			Type:    "UNICODE_BYPASS_BLOCKED",
			Details: map[string]interface{}{"hostname": hostname},
		}
	}

	return nil
}

// hasMixedScripts detects script mixing across Latin and Cyrillic character sets.
func hasMixedScripts(hostname string) bool {
	var latinCount, cyrillicCount, otherCount int

	for _, r := range hostname {
		if unicode.Is(unicode.Latin, r) {
			latinCount++
		} else if unicode.Is(unicode.Cyrillic, r) {
			cyrillicCount++
		} else if unicode.IsLetter(r) {
			otherCount++
		}
	}

	scriptsFound := 0
	if latinCount > 0 {
		scriptsFound++
	}
	if cyrillicCount > 0 {
		scriptsFound++
	}
	if otherCount > 0 {
		scriptsFound++
	}

	return scriptsFound > 1
}

// hasConfusableChars detects confusable Unicode characters mimicking ASCII.
func hasConfusableChars(hostname string) bool {
	normalized := norm.NFKC.String(hostname)

	confusables := map[rune]rune{
		'а': 'a',
		'е': 'e',
		'о': 'o',
		'р': 'p',
		'с': 'c',
		'х': 'x',
	}

	for cyrillic := range confusables {
		if strings.ContainsRune(normalized, cyrillic) {
			return true
		}
	}

	return false
}

// isSuspiciousDomain identifies domains prone to DNS rebinding or TOCTOU attacks.
func isSuspiciousDomain(hostname string) bool {
	h := strings.ToLower(hostname)

	suspiciousPatterns := []string{
		"toctou", "rebind", "attack", "malicious", "evil",
		"127.", "10.", "192.168.", "172.16.", "172.17.",
		"localhost", "internal", "private", "test",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(h, pattern) {
			return true
		}
	}

	suspiciousTLDs := []string{
		".tk", ".ml", ".ga", ".cf",
		".temp", ".tmp", ".test",
		".nip.io",
	}

	for _, tld := range suspiciousTLDs {
		if strings.HasSuffix(h, tld) {
			return true
		}
	}

	return false
}
