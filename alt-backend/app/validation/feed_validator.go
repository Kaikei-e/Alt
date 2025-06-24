package validation

import (
	"context"
	"net"
	"net/url"
	"strings"
)

type FeedRegistrationValidator struct{}

func (v *FeedRegistrationValidator) Validate(ctx context.Context, value interface{}) ValidationResult {
	result := ValidationResult{Valid: true}
	
	// Check if input is a map (JSON object)
	inputMap, ok := value.(map[string]interface{})
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "body",
			Message: "Request body must be a valid object",
		})
		return result
	}
	
	// Check if URL field exists
	urlField, exists := inputMap["url"]
	if !exists {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "URL field is required",
		})
		return result
	}
	
	// Check if URL is a string
	urlStr, ok := urlField.(string)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "URL must be a string",
		})
		return result
	}
	
	// Validate URL using FeedURLValidator
	urlValidator := &FeedURLValidator{}
	urlResult := urlValidator.Validate(ctx, urlStr)
	if !urlResult.Valid {
		result.Valid = false
		result.Errors = append(result.Errors, urlResult.Errors...)
		return result
	}
	
	// Additional SSRF protection checks
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		// This should not happen as FeedURLValidator already checked it
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Invalid URL format",
			Value:   urlStr,
		})
		return result
	}
	
	// Check for localhost
	hostname := strings.ToLower(parsedURL.Hostname())
	if hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.") {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Access to localhost not allowed for security reasons",
			Value:   urlStr,
		})
		return result
	}
	
	// Check for metadata endpoints
	if hostname == "169.254.169.254" || hostname == "metadata.google.internal" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Access to metadata endpoints not allowed for security reasons",
			Value:   urlStr,
		})
		return result
	}
	
	// Check for private IP addresses
	if isPrivateIPAddress(hostname) {
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

type FeedDetailValidator struct{}

func (v *FeedDetailValidator) Validate(ctx context.Context, value interface{}) ValidationResult {
	result := ValidationResult{Valid: true}
	
	// Check if input is a map (JSON object)
	inputMap, ok := value.(map[string]interface{})
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "body",
			Message: "Request body must be a valid object",
		})
		return result
	}
	
	// Check if feed_url field exists
	feedURLField, exists := inputMap["feed_url"]
	if !exists {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "feed_url",
			Message: "feed_url field is required",
		})
		return result
	}
	
	// Check if feed_url is a string
	feedURLStr, ok := feedURLField.(string)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "feed_url",
			Message: "feed_url must be a string",
		})
		return result
	}
	
	// Check if feed_url is empty
	if strings.TrimSpace(feedURLStr) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "feed_url",
			Message: "Feed URL cannot be empty",
			Value:   feedURLStr,
		})
		return result
	}
	
	// Validate feed URL format
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

// Helper function to check if hostname resolves to private IP
func isPrivateIPAddress(hostname string) bool {
	// Try to parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		return isPrivateIP(ip)
	}
	
	// If it's a hostname, resolve it to IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Block on resolution failure as a security measure
		return true
	}
	
	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true
		}
	}
	
	return false
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	
	// Check for private IPv4 ranges
	ipv4 := ip.To4()
	if ipv4 != nil {
		// 10.0.0.0/8
		if ipv4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ipv4[0] == 172 && ipv4[1] >= 16 && ipv4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ipv4[0] == 192 && ipv4[1] == 168 {
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