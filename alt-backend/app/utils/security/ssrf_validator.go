package security

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// SSRFValidator provides comprehensive SSRF protection with DNS rebinding prevention
type SSRFValidator struct {
	allowedDomains        []string
	metadataEndpoints     []string
	internalDomains       []string
	allowedPorts          map[string]bool
	dnsCache              map[string][]net.IP
	cacheTTL              time.Duration
	allowTestingLocalhost bool
}

// ValidationError represents a validation error with context
type ValidationError struct {
	Message string
	Type    string
	Details map[string]interface{}
}

// Error formats the validation error for display.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// NewSSRFValidator creates a new SSRF validator with default security settings.
// allowedDomains is intentionally left empty: callers that need an allow-list
// should opt in explicitly. The previous placeholder ("example.com") was dead
// code that risked giving readers a false sense of safety (M-003).
func NewSSRFValidator() *SSRFValidator {
	return &SSRFValidator{
		allowedDomains:    nil,
		metadataEndpoints: append([]string(nil), metadataEndpointsList...),
		internalDomains:   append([]string(nil), defaultInternalDomains...),
		// L-004: only the IANA standard HTTP/HTTPS ports are accepted by
		// default. Non-standard ports (8080, 8443, etc.) are common targets
		// for internal services so we no longer allow them implicitly. If a
		// caller needs them they can append via SSRFValidator.allowedPorts
		// after construction.
		allowedPorts: map[string]bool{
			"80":  true,
			"443": true,
		},
		dnsCache:              make(map[string][]net.IP),
		cacheTTL:              5 * time.Minute,
		allowTestingLocalhost: false,
	}
}

// SetTestingMode enables testing mode that allows localhost
func (v *SSRFValidator) SetTestingMode(enabled bool) {
	v.allowTestingLocalhost = enabled
}

// CanonicalRequestURL validates u under the SSRF policy and returns a reconstructed
// URL string built only from allowed components (scheme, host, path, query).
// Callers must use the returned string (not the original raw input) for http.NewRequest.
func (v *SSRFValidator) CanonicalRequestURL(ctx context.Context, u *url.URL) (string, error) {
	if u == nil {
		return "", &ValidationError{Message: "URL is nil", Type: "NIL_URL"}
	}
	if err := v.ValidateURL(ctx, u); err != nil {
		return "", err
	}
	scheme, err := allowlistedRequestScheme(u.Scheme)
	if err != nil {
		return "", err
	}

	// Path carries the decoded form and RawPath its original encoding;
	// setting Path to EscapedPath() would make String() escape it a second
	// time (%2C -> %252C), corrupting signed CDN URLs.
	// Scheme is an allowlisted constant (http/https); Host/Path/Query are
	// copied only after ValidateURL. Userinfo, fragment, and opaque are dropped.
	safe := &url.URL{
		Scheme:   scheme,
		Host:     u.Host,
		Path:     u.Path,
		RawPath:  u.RawPath,
		RawQuery: u.RawQuery,
	}
	if safe.Path == "" {
		safe.Path = "/"
	}
	canonical := safe.String()
	// Match the reconstructed string (same SSA value we return). go/request-forgery
	// treats regexp.MatchString as a URL sanitizer; returning a different expression
	// would leave taint on the caller's request URL.
	if !canonicalRequestURLRe.MatchString(canonical) {
		return "", &ValidationError{
			Message: "reconstructed URL is not a canonical http(s) request URL",
			Type:    "CANONICAL_URL_REJECTED",
		}
	}
	return canonical, nil
}

// ValidateURL performs comprehensive URL validation including DNS rebinding protection
func (v *SSRFValidator) ValidateURL(ctx context.Context, u *url.URL) error {
	if err := v.basicValidation(u); err != nil {
		return err
	}

	if u.User != nil {
		return &ValidationError{
			Message: "URLs with user info are not allowed",
			Type:    "USER_INFO_BLOCKED",
		}
	}

	if err := v.validateScheme(u); err != nil {
		return err
	}

	if err := v.validateHost(u); err != nil {
		return err
	}

	if err := v.validatePath(u); err != nil {
		return err
	}

	if err := v.validatePorts(u); err != nil {
		return err
	}

	if err := v.validateUnicodeAndPunycode(u); err != nil {
		return err
	}

	if err := v.validateDNSRebinding(ctx, u); err != nil {
		return err
	}

	return nil
}

// basicValidation performs basic URL structure validation
func (v *SSRFValidator) basicValidation(u *url.URL) error {
	if u == nil {
		return &ValidationError{
			Message: "URL cannot be nil",
			Type:    "BASIC_VALIDATION_ERROR",
		}
	}

	if u.Host == "" {
		return &ValidationError{
			Message: "empty host not allowed",
			Type:    "BASIC_VALIDATION_ERROR",
		}
	}

	return nil
}

// validateScheme ensures only HTTP/HTTPS are allowed
func (v *SSRFValidator) validateScheme(u *url.URL) error {
	if u.Scheme != "https" && u.Scheme != "http" {
		return &ValidationError{
			Message: "only HTTP and HTTPS schemes allowed",
			Type:    "SCHEME_VALIDATION_ERROR",
		}
	}
	return nil
}

// validateHost performs hostname validation including metadata and internal domain checks
func (v *SSRFValidator) validateHost(u *url.URL) error {
	hostname := strings.ToLower(u.Hostname())

	// Check metadata endpoints first (highest priority)
	for _, endpoint := range v.metadataEndpoints {
		if hostname == endpoint || strings.HasPrefix(hostname, endpoint+":") {
			return &ValidationError{
				Message: "access to metadata endpoint not allowed",
				Type:    "METADATA_ENDPOINT_BLOCKED",
				Details: map[string]interface{}{
					"hostname": hostname,
					"endpoint": endpoint,
				},
			}
		}
	}

	// Check internal domains
	for _, domainSuffix := range v.internalDomains {
		if strings.HasSuffix(hostname, domainSuffix) {
			return &ValidationError{
				Message: "access to internal domains not allowed",
				Type:    "INTERNAL_DOMAIN_BLOCKED",
				Details: map[string]interface{}{
					"hostname": hostname,
					"suffix":   domainSuffix,
				},
			}
		}
	}

	return nil
}

// validatePath checks for path traversal attacks
func (v *SSRFValidator) validatePath(u *url.URL) error {
	return validatePathSyntax(u.Path, u.RawPath)
}

// validatePorts ensures only allowed ports are used
func (v *SSRFValidator) validatePorts(u *url.URL) error {
	return validatePortNumber(u.Port(), v.allowedPorts, v.allowTestingLocalhost, u.Hostname())
}

// validateUnicodeAndPunycode checks for Unicode/Punycode bypass attempts
func (v *SSRFValidator) validateUnicodeAndPunycode(u *url.URL) error {
	return validateUnicodeAndPunycode(u.Hostname(), v.allowTestingLocalhost)
}
