package image_fetch_gateway

import (
	"alt/domain"
	"alt/utils/errors"
	"alt/utils/logger"
	"alt/utils/proxy"
	"alt/utils/security"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// allowedProxyHosts defines known safe proxy hosts that may be used for image fetching.
// This prevents unexpected hosts from being targeted when proxy mode is enabled.
var allowedProxyHosts = map[string]struct{}{
	"envoy-proxy.alt-apps.svc.cluster.local:8085": {},
	"envoy-proxy.alt-apps.svc.cluster.local:8080": {},
}

// ImageFetchGateway implements the ImageFetchPort interface
// It acts as an Anti-Corruption Layer between the domain and external HTTP APIs
type ImageFetchGateway struct {
	httpClient    *http.Client
	proxyStrategy *proxy.Strategy
	ssrfValidator *security.SSRFValidator
}

// NewImageFetchGateway creates a new ImageFetchGateway
func NewImageFetchGateway(httpClient *http.Client) *ImageFetchGateway {
	strategy := proxy.GetStrategy()

	// Create SSRF validator with comprehensive protection
	ssrfValidator := security.NewSSRFValidator()

	// Use secure HTTP client from validator instead of modifying existing client
	// This follows the Safeurl approach with connection-time validation
	var secureClient *http.Client
	if httpClient != nil {
		timeout := httpClient.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		secureClient = ssrfValidator.CreateSecureHTTPClient(timeout)
	} else {
		secureClient = ssrfValidator.CreateSecureHTTPClient(30 * time.Second)
	}

	// Create the gateway instance
	gateway := &ImageFetchGateway{
		httpClient:    secureClient,
		proxyStrategy: strategy,
		ssrfValidator: ssrfValidator,
	}

	// If we have a proxy strategy, we need to modify the transport to include proxy support
	if strategy != nil && strategy.Enabled {
		// Get the current secure transport
		secureTransport := secureClient.Transport

		// Wrap with Envoy proxy round tripper using shared proxy package
		secureClient.Transport = proxy.WrapTransportForProxy(secureTransport, strategy)
	}

	return gateway
}

// validateImageURLWithTestOverride allows bypassing localhost restrictions for testing
// Enhanced with additional SSRF protection measures
func validateImageURLWithTestOverride(u *url.URL, allowTestingLocalhost bool) error {
	// Only allow HTTPS and HTTP
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("only HTTP and HTTPS schemes allowed")
	}

	// Validate URL format and prevent malformed URLs
	if u.Host == "" {
		return fmt.Errorf("empty host not allowed")
	}

	// Check for dangerous path patterns that could indicate path traversal attacks
	if strings.Contains(u.Path, "..") || strings.Contains(u.Path, "/.") {
		return fmt.Errorf("path traversal patterns not allowed")
	}

	// Check for URL encoding attacks by examining the raw URL.
	// Only block control characters and backslash encoding — encoded dots (%2e) and
	// slashes (%2f) are NOT blocked because CDN URLs legitimately use them, and path
	// traversal via %2e%2e is already caught by the decoded-path ".." check above.
	rawURL := u.String()
	if strings.Contains(rawURL, "%5c") || strings.Contains(rawURL, "%5C") ||
		strings.Contains(rawURL, "%00") ||
		strings.Contains(rawURL, "%0a") || strings.Contains(rawURL, "%0A") ||
		strings.Contains(rawURL, "%0d") || strings.Contains(rawURL, "%0D") {
		return fmt.Errorf("URL encoding attacks not allowed")
	}

	// Check hostname and perform security checks
	hostname := strings.ToLower(u.Hostname())
	isTestingLocalhost := allowTestingLocalhost && (hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127."))

	// Enhanced: Block cloud metadata endpoints (security priority over domain allowlist)
	metadataEndpoints := []string{
		"169.254.169.254",          // AWS/Azure metadata
		"metadata.google.internal", // GCP metadata
		"100.100.100.200",          // Alibaba Cloud
		"169.254.169.254:80",       // Explicit port
		"169.254.169.254:8080",     // Alternative ports
	}
	for _, endpoint := range metadataEndpoints {
		if hostname == endpoint || strings.HasPrefix(hostname, endpoint+":") {
			return fmt.Errorf("access to metadata endpoint not allowed")
		}
	}

	// Enhanced: Block internal domains (security priority over domain allowlist)
	internalDomains := []string{".local", ".internal", ".corp", ".lan", ".intranet", ".test", ".localhost"}
	for _, domainSuffix := range internalDomains {
		if strings.HasSuffix(hostname, domainSuffix) {
			return fmt.Errorf("access to internal domains not allowed")
		}
	}

	// Block private networks (except localhost for testing when allowed)
	if !isTestingLocalhost && isPrivateIP(u.Hostname()) {
		return fmt.Errorf("access to private networks not allowed")
	}

	// Block localhost variations (unless testing is allowed)
	if !isTestingLocalhost && (hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.")) {
		return fmt.Errorf("access to localhost not allowed")
	}

	// Check domain whitelist (after security checks)
	if !domain.IsAllowedImageDomain(hostname) && !isTestingLocalhost {
		return fmt.Errorf("domain not in whitelist")
	}

	// Block URLs with non-standard ports that could be used for port scanning
	if u.Port() != "" {
		port := u.Port()
		allowedPorts := map[string]bool{"80": true, "443": true, "8080": true, "8443": true}
		if !allowedPorts[port] {
			return fmt.Errorf("non-standard port not allowed: %s", port)
		}
	}

	return nil
}

// isPrivateIP checks if the hostname resolves to private IP addresses
// This function delegates to security.IsPrivateHost for centralized IP validation
func isPrivateIP(hostname string) bool {
	return security.IsPrivateHost(hostname)
}

// FetchImage fetches an image from external URL through HTTP client
func (g *ImageFetchGateway) FetchImage(ctx context.Context, imageURL *url.URL, options *domain.ImageFetchOptions) (*domain.ImageFetchResult, error) {
	return g.fetchImageWithTestingOverride(ctx, imageURL, options, false)
}

// fetchImageForTesting allows bypassing localhost restrictions for unit testing
func (g *ImageFetchGateway) fetchImageForTesting(ctx context.Context, imageURL *url.URL, options *domain.ImageFetchOptions) (*domain.ImageFetchResult, error) {
	// Enable testing mode in the SSRF validator
	g.ssrfValidator.SetTestingMode(true)
	defer g.ssrfValidator.SetTestingMode(false)

	return g.fetchImageWithTestingOverride(ctx, imageURL, options, true)
}

// fetchImageWithTestingOverride is the internal implementation with testing override capability
func (g *ImageFetchGateway) fetchImageWithTestingOverride(ctx context.Context, imageURL *url.URL, options *domain.ImageFetchOptions, allowTestingLocalhost bool) (*domain.ImageFetchResult, error) {
	// Check context cancellation early
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// SSRF Protection: Comprehensive multi-layer validation performed here
	// First layer: Pre-request validation at URL parsing time
	// - Validates URL scheme (HTTP/HTTPS only)
	// - Blocks cloud metadata endpoints (AWS/GCP/Azure/Alibaba/Oracle)
	// - Blocks private IP ranges (RFC1918: 10.x, 172.16-31.x, 192.168.x)
	// - Blocks loopback and link-local addresses
	// - Validates DNS resolution to prevent DNS rebinding attacks
	// - Blocks internal domain suffixes (.local, .internal, .cluster.local, etc)
	// - Validates ports (only 80, 443, 8080, 8443 allowed)
	// - Prevents path traversal and Unicode/Punycode bypass attacks
	if err := g.ssrfValidator.ValidateURL(ctx, imageURL); err != nil {
		return nil, errors.NewValidationContextError(
			fmt.Sprintf("URL validation failed: %v", err),
			"gateway",
			"ImageFetchGateway",
			"validate_url",
			map[string]interface{}{
				"url": imageURL.String(),
			},
		)
	}

	// Convert to proxy URL if proxy strategy is enabled using shared proxy package
	requestURL := imageURL.String()
	if g.proxyStrategy != nil && g.proxyStrategy.Enabled {
		requestURL = proxy.ConvertToProxyURLWithContext(ctx, imageURL.String(), g.proxyStrategy)
		logger.SafeInfoContext(ctx, "Using proxy strategy for image fetch",
			"strategy_mode", string(g.proxyStrategy.Mode),
			"original_url", imageURL.String(),
			"proxy_url", requestURL)
	} else {
		logger.SafeInfoContext(ctx, "Using direct connection for image fetch (no proxy configured)",
			"original_url", imageURL.String())
	}

	// Parse and validate the final request URL to guard against SSRF
	parsedReqURL, err := url.Parse(requestURL)
	if err != nil {
		return nil, errors.NewValidationContextError(
			fmt.Sprintf("invalid request URL: %v", err),
			"gateway",
			"ImageFetchGateway",
			"parse_request_url",
			map[string]interface{}{
				"url": requestURL,
			},
		)
	}
	// Validate proxy configuration if using proxy
	if g.proxyStrategy != nil && g.proxyStrategy.Enabled {
		proxyBase, err := url.Parse(g.proxyStrategy.BaseURL)
		if err != nil {
			return nil, errors.NewValidationContextError(
				"invalid proxy base URL",
				"gateway",
				"ImageFetchGateway",
				"validate_proxy_host",
				map[string]interface{}{
					"base_url": g.proxyStrategy.BaseURL,
				},
			)
		}
		proxyHost := strings.ToLower(proxyBase.Host)
		if _, ok := allowedProxyHosts[proxyHost]; !ok || !strings.EqualFold(parsedReqURL.Host, proxyBase.Host) {
			return nil, errors.NewValidationContextError(
				"proxy host not allowed",
				"gateway",
				"ImageFetchGateway",
				"validate_proxy_host",
				map[string]interface{}{
					"host": proxyHost,
				},
			)
		}
	}
	// SSRF Protection: Second layer - Connection-time validation
	// The httpClient (created by SSRFValidator.CreateSecureHTTPClient) performs real-time
	// IP validation at the syscall.Control hook level during actual connection establishment.
	// This prevents DNS rebinding attacks where DNS resolves to a safe IP during validation
	// but changes to a dangerous IP before connection. All redirects are also blocked.
	// See: SSRFValidator.validateConnectionAddress() for implementation details.

	// Create HTTP request with proper headers
	req, err := http.NewRequestWithContext(ctx, "GET", parsedReqURL.String(), nil)
	if err != nil {
		return nil, errors.NewExternalAPIContextError(
			"failed to create HTTP request",
			"gateway",
			"ImageFetchGateway",
			"create_request",
			err,
			map[string]interface{}{
				"url": imageURL.String(),
			},
		)
	}

	// Set appropriate headers for image fetching
	req.Header.Set("User-Agent", "Alt-RSS-Reader/1.0 (+https://alt.example.com)")
	req.Header.Set("Accept", "image/webp, image/jpeg, image/png, image/gif")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Cache-Control", "no-cache")

	// SSRF Protection Summary: Multi-layer defense implemented
	// Layer 1: URL validation at lines 393-413 (SSRFValidator.ValidateURL)
	// Layer 2: Proxy allowlist validation at lines 441-467 (if proxy enabled)
	// Layer 3: Connection-time IP validation (via secure httpClient created at line 207)
	// Layer 4: Redirect blocking (httpClient.CheckRedirect blocks all redirects)
	// codeql[go/request-forgery] - False positive: URL validated by comprehensive SSRF protection
	resp, err := g.httpClient.Do(req)
	if err != nil {
		// Check if it's a timeout error
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, errors.NewTimeoutContextError(
				"request timeout",
				"gateway",
				"ImageFetchGateway",
				"http_request",
				err,
				map[string]interface{}{
					"url":     imageURL.String(),
					"timeout": options.Timeout.String(),
				},
			)
		}

		return nil, errors.NewExternalAPIContextError(
			"HTTP request failed",
			"gateway",
			"ImageFetchGateway",
			"http_request",
			err,
			map[string]interface{}{
				"url": imageURL.String(),
			},
		)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	// Check HTTP status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.NewExternalAPIContextError(
			fmt.Sprintf("HTTP request failed with status %d", resp.StatusCode),
			"gateway",
			"ImageFetchGateway",
			"http_response",
			fmt.Errorf("status code: %d", resp.StatusCode),
			map[string]interface{}{
				"url":         imageURL.String(),
				"status_code": resp.StatusCode,
				"status":      resp.Status,
			},
		)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	if !domain.IsValidImageContentType(contentType) {
		return nil, errors.NewValidationContextError(
			"response is not an image",
			"gateway",
			"ImageFetchGateway",
			"validate_content_type",
			map[string]interface{}{
				"url":          imageURL.String(),
				"content_type": contentType,
			},
		)
	}

	// Check content length if available
	contentLengthHeader := resp.Header.Get("Content-Length")
	if contentLengthHeader != "" {
		if contentLength, err := strconv.ParseInt(contentLengthHeader, 10, 64); err == nil {
			// Safe comparison with bounds checking to prevent integer overflow
			maxSizeInt64 := int64(options.MaxSize)

			// Check if content length exceeds int32 bounds or the configured max size
			if contentLength > math.MaxInt32 || contentLength > maxSizeInt64 {
				return nil, errors.NewValidationContextError(
					"image too large",
					"gateway",
					"ImageFetchGateway",
					"validate_size",
					map[string]interface{}{
						"url":            imageURL.String(),
						"content_length": contentLength,
						"max_size":       options.MaxSize,
					},
				)
			}
		}
	}

	// Read the response body with size limit
	limitedReader := io.LimitReader(resp.Body, int64(options.MaxSize+1)) // +1 to detect if it exceeds
	imageData, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, errors.NewExternalAPIContextError(
			"failed to read response body",
			"gateway",
			"ImageFetchGateway",
			"read_response",
			err,
			map[string]interface{}{
				"url": imageURL.String(),
			},
		)
	}

	// Check actual size
	if len(imageData) > options.MaxSize {
		return nil, errors.NewValidationContextError(
			"image too large",
			"gateway",
			"ImageFetchGateway",
			"validate_actual_size",
			map[string]interface{}{
				"url":         imageURL.String(),
				"actual_size": len(imageData),
				"max_size":    options.MaxSize,
			},
		)
	}

	// Create and return the result
	result := &domain.ImageFetchResult{
		URL:         imageURL.String(),
		ContentType: contentType,
		Data:        imageData,
		Size:        len(imageData),
		FetchedAt:   time.Now(),
	}

	return result, nil
}
