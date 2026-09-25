package validation

import (
	"alt/utils/security"
	"context"
	"net"
	"net/url"
	"strings"
)

// IPResolver defines the lookup signature for resolving hostnames to IP addresses.
type IPResolver func(host string) ([]net.IP, error)

// FeedRegistrationValidator validates feed registration requests with syntactic and SSRF checks.
type FeedRegistrationValidator struct {
	// Resolver allows injecting a custom DNS resolver for testing and network decoupling.
	Resolver IPResolver
}

// Validate checks feed registration payloads for structure, syntax, and SSRF threats.
func (v *FeedRegistrationValidator) Validate(ctx context.Context, value interface{}) ValidationResult {
	result := ValidationResult{Valid: true}

	// Ensure the request payload is a JSON object map.
	inputMap, ok := value.(map[string]interface{})
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "body",
			Message: "Request body must be a valid object",
		})
		return result
	}

	// Ensure the URL field is present in the payload.
	urlField, exists := inputMap["url"]
	if !exists {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "URL field is required",
		})
		return result
	}

	// Ensure the URL field is a string.
	urlStr, ok := urlField.(string)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "URL must be a string",
		})
		return result
	}

	// Validate syntactic URL format using FeedURLValidator.
	urlValidator := &FeedURLValidator{}
	urlResult := urlValidator.Validate(ctx, urlStr)
	if !urlResult.Valid {
		result.Valid = false
		result.Errors = append(result.Errors, urlResult.Errors...)
		return result
	}

	// Parse URL for domain and SSRF policy inspection.
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Invalid URL format",
			Value:   urlStr,
		})
		return result
	}

	// Allow explicitly configured operator feed hosts.
	hostname := strings.ToLower(parsedURL.Hostname())
	if security.IsFeedHostAllowed(hostname) {
		return result
	}

	// Block loopback and localhost hostnames.
	if hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.") {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Access to localhost not allowed for security reasons",
			Value:   urlStr,
		})
		return result
	}

	// Block cloud metadata hostnames using consolidated definitions.
	if security.IsMetadataHost(hostname) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Access to metadata endpoints not allowed for security reasons",
			Value:   urlStr,
		})
		return result
	}

	// Block restricted internal domain suffixes.
	if strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".internal") ||
		strings.HasSuffix(hostname, ".corp") || strings.HasSuffix(hostname, ".lan") {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "internal domain suffixes not allowed",
			Value:   urlStr,
		})
		return result
	}

	// Block unspecified host 0.0.0.0.
	if hostname == "0.0.0.0" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Access to private networks not allowed for security reasons",
			Value:   urlStr,
		})
		return result
	}

	// Block private IP literals directly without performing DNS resolution.
	ip := net.ParseIP(hostname)
	if ip != nil {
		if security.IsPrivateIPAddress(ip) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   "url",
				Message: "Access to private networks not allowed for security reasons",
				Value:   urlStr,
			})
			return result
		}
		return result
	}

	// Resolve hostname to check for private network targets.
	if v.isPrivateHost(hostname) {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Access to private networks not allowed for security reasons",
			Value:   urlStr,
		})
		return result
	}

	return result
}

// isPrivateHost resolves a hostname via the injected or default resolver and checks for private IPs.
func (v *FeedRegistrationValidator) isPrivateHost(hostname string) bool {
	resolver := v.Resolver
	if resolver == nil {
		resolver = net.LookupIP
	}
	return resolveAndCheckPrivate(hostname, resolver)
}

// resolveAndCheckPrivate performs DNS lookup using the given resolver and checks against private IP ranges.
func resolveAndCheckPrivate(hostname string, resolver IPResolver) bool {
	commonTLDs := []string{".com", ".org", ".net", ".edu", ".gov", ".mil", ".int"}
	isCommonTLD := false
	for _, tld := range commonTLDs {
		if strings.HasSuffix(strings.ToLower(hostname), tld) {
			isCommonTLD = true
			break
		}
	}

	ips, err := resolver(hostname)
	if err != nil {
		// Reject uncommon TLDs on DNS failure while allowing common TLDs to handle transient DNS issues.
		if !isCommonTLD {
			return true
		}
		return false
	}

	for _, resolvedIP := range ips {
		if security.IsPrivateIPAddress(resolvedIP) {
			return true
		}
	}

	return false
}

// FeedDetailValidator validates detail request objects for feeds.
type FeedDetailValidator struct{}

// Validate verifies that feed detail payload contains a valid HTTP or HTTPS feed URL.
func (v *FeedDetailValidator) Validate(ctx context.Context, value interface{}) ValidationResult {
	result := ValidationResult{Valid: true}

	inputMap, ok := value.(map[string]interface{})
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "body",
			Message: "Request body must be a valid object",
		})
		return result
	}

	feedURLField, exists := inputMap["feed_url"]
	if !exists {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "feed_url",
			Message: "feed_url field is required",
		})
		return result
	}

	feedURLStr, ok := feedURLField.(string)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "feed_url",
			Message: "feed_url must be a string",
		})
		return result
	}

	if strings.TrimSpace(feedURLStr) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "feed_url",
			Message: "Feed URL cannot be empty",
			Value:   feedURLStr,
		})
		return result
	}

	parsedURL, err := url.Parse(feedURLStr)
	if err != nil || parsedURL.Scheme == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "feed_url",
			Message: "Invalid feed URL format",
			Value:   feedURLStr,
		})
		return result
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "feed_url",
			Message: "Feed URL must use HTTP or HTTPS scheme",
			Value:   feedURLStr,
		})
		return result
	}

	return result
}
