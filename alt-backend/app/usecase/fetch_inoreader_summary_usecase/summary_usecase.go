package fetch_inoreader_summary_usecase

import (
	"alt/domain"
	"alt/port/fetch_inoreader_summary_port"
	"alt/utils/logger"
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type FetchInoreaderSummaryUsecase interface {
	Execute(ctx context.Context, urls []string) ([]*domain.InoreaderSummary, error)
}

type fetchInoreaderSummaryUsecase struct {
	port fetch_inoreader_summary_port.FetchInoreaderSummaryPort
}

// NewFetchInoreaderSummaryUsecase creates a new usecase instance
func NewFetchInoreaderSummaryUsecase(port fetch_inoreader_summary_port.FetchInoreaderSummaryPort) FetchInoreaderSummaryUsecase {
	return &fetchInoreaderSummaryUsecase{
		port: port,
	}
}

// Execute fetches inoreader summaries for the provided URLs
func (u *fetchInoreaderSummaryUsecase) Execute(ctx context.Context, urls []string) ([]*domain.InoreaderSummary, error) {
	logger.Logger.InfoContext(ctx, "Usecase: fetching inoreader summaries",
		"url_count", len(urls),
		"urls", urls)

	// Validation: Check URL count limits (as per schema validation max=50)
	if len(urls) > 50 {
		logger.Logger.ErrorContext(ctx, "Too many URLs provided", "count", len(urls), "max", 50)
		return nil, fmt.Errorf("too many URLs: maximum 50 allowed, got %d", len(urls))
	}

	// Handle empty input
	if len(urls) == 0 {
		logger.Logger.InfoContext(ctx, "No URLs provided, returning empty result")
		return []*domain.InoreaderSummary{}, nil
	}

	// SSRF Protection: Validate all URLs for security
	if err := u.validateURLsForSecurity(urls); err != nil {
		logger.Logger.ErrorContext(ctx, "URL security validation failed", "error", err, "url_count", len(urls))
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Remove duplicates while preserving order
	uniqueURLs := u.removeDuplicateURLs(urls)

	logger.Logger.InfoContext(ctx, "URLs processed",
		"original_count", len(urls),
		"unique_count", len(uniqueURLs))

	// Call port (gateway layer)
	summaries, err := u.port.FetchSummariesByURLs(ctx, uniqueURLs)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Port layer failed", "error", err, "url_count", len(uniqueURLs))
		return nil, fmt.Errorf("failed to fetch summaries: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Usecase: successfully fetched summaries",
		"matched_count", len(summaries),
		"requested_count", len(uniqueURLs))

	return summaries, nil
}

// validateURLsForSecurity validates URLs for SSRF protection (same pattern as handler.go)
func (u *fetchInoreaderSummaryUsecase) validateURLsForSecurity(urls []string) error {
	for _, urlStr := range urls {
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return fmt.Errorf("invalid URL format '%s': %w", urlStr, err)
		}

		if err := u.isAllowedURL(parsedURL); err != nil {
			return fmt.Errorf("URL '%s' not allowed: %w", urlStr, err)
		}
	}
	return nil
}

// isAllowedURL validates a URL for SSRF protection (copied from handler.go pattern)
func (u *fetchInoreaderSummaryUsecase) isAllowedURL(parsedURL *url.URL) error {
	// Allow both HTTP and HTTPS
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return stderrors.New("only HTTP and HTTPS schemes allowed")
	}

	// Block private networks
	if u.isPrivateIP(parsedURL.Hostname()) {
		return stderrors.New("access to private networks not allowed")
	}

	// Block localhost variations
	hostname := strings.ToLower(parsedURL.Hostname())
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

// isPrivateIP checks if hostname resolves to private IP addresses
func (u *fetchInoreaderSummaryUsecase) isPrivateIP(hostname string) bool {
	// Try to parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		return u.isPrivateIPAddress(ip)
	}

	// If it's a hostname, resolve it to IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Block on resolution failure as a security measure
		return true
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if u.isPrivateIPAddress(ip) {
			return true
		}
	}

	return false
}

// isPrivateIPAddress checks if an IP address is private
func (u *fetchInoreaderSummaryUsecase) isPrivateIPAddress(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	if ipv4 := ip.To4(); ipv4 != nil {
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

// removeDuplicateURLs removes duplicate URLs while preserving order
func (u *fetchInoreaderSummaryUsecase) removeDuplicateURLs(urls []string) []string {
	seen := make(map[string]bool)
	unique := make([]string, 0, len(urls))

	for _, url := range urls {
		if !seen[url] {
			seen[url] = true
			unique = append(unique, url)
		}
	}

	return unique
}
