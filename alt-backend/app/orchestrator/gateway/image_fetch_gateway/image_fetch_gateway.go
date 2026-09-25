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
	"net/http"
	"net/url"
	"strings"
	"time"
)

// validateResponseEncoding rejects responses whose body is still in an
// encoding Go's net/http Transport did not transparently decompress. When the
// Transport auto-requests gzip and the server returns it, the Content-Encoding
// header is stripped from the response — so any remaining non-empty value
// (other than "identity") means the body is compressed with an algorithm we
// cannot decode (br/zstd/deflate/etc.), and passing those bytes to the image
// decoder would fail opaquely.
func validateResponseEncoding(resp *http.Response) error {
	return checkContentEncoding(resp.Header.Get("Content-Encoding"))
}

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
	// Validate proxy configuration if using proxy; otherwise re-validate the final request URL.
	var safeReqURL string
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
		// Proxy host is allowlisted; still drop userinfo/fragment via reconstruction.
		// Path/RawPath keep the original encoding — assigning EscapedPath() to
		// Path would double-escape percent-encoded segments (%2C -> %252C).
		safeReqURL = (&url.URL{
			Scheme:   strings.ToLower(parsedReqURL.Scheme),
			Host:     parsedReqURL.Host,
			Path:     parsedReqURL.Path,
			RawPath:  parsedReqURL.RawPath,
			RawQuery: parsedReqURL.RawQuery,
		}).String()
	} else {
		canonical, err := g.ssrfValidator.CanonicalRequestURL(ctx, parsedReqURL)
		if err != nil {
			return nil, errors.NewValidationContextError(
				fmt.Sprintf("request URL validation failed: %v", err),
				"gateway",
				"ImageFetchGateway",
				"validate_request_url",
				map[string]interface{}{
					"url": parsedReqURL.String(),
				},
			)
		}
		safeReqURL = canonical
	}

	// Create HTTP request with proper headers
	req, err := http.NewRequestWithContext(ctx, "GET", safeReqURL, nil)
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

	// Set appropriate headers for image fetching.
	// NOTE: We intentionally do not set Accept-Encoding. Go's http.Transport
	// auto-adds "Accept-Encoding: gzip" and transparently decompresses the
	// response body when the Transport — not the user — requested gzip
	// (see https://pkg.go.dev/net/http#Transport). Setting it manually here
	// disables that transparent decompression, which previously caused
	// "image: unknown format" errors when upstream CDNs (e.g., Dezeen) served
	// gzip-encoded JPEGs. The existing io.LimitReader below bounds the
	// decompressed size, so gzip bombs are still contained.
	req.Header.Set("User-Agent", "Alt-RSS-Reader/1.0 (+https://alt.example.com)")
	req.Header.Set("Accept", "image/webp, image/jpeg, image/png, image/gif")
	req.Header.Set("Cache-Control", "no-cache")

	// SSRF: CanonicalRequestURL / proxy allowlist + CreateSecureHTTPClient dial checks.
	// codeql[go/request-forgery] - URL reconstructed by SSRFValidator.CanonicalRequestURL or proxy allowlist
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

	// Reject responses whose body remains in an encoding Go did not decompress
	// (br/zstd/etc.). Must run before reading the body so we do not consume
	// bytes we cannot interpret.
	if err := validateResponseEncoding(resp); err != nil {
		return nil, errors.NewValidationContextError(
			"response encoding not supported",
			"gateway",
			"ImageFetchGateway",
			"validate_content_encoding",
			map[string]interface{}{
				"url":              imageURL.String(),
				"content_encoding": resp.Header.Get("Content-Encoding"),
			},
		)
	}

	// Validate content type and content length headers
	contentType := resp.Header.Get("Content-Type")
	contentLengthHeader := resp.Header.Get("Content-Length")
	contentLength, err := validateImageHeaders(contentType, contentLengthHeader, options.MaxSize)
	if err != nil {
		if err == errInvalidContentType {
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
	return buildImageFetchResult(imageURL.String(), contentType, imageData, time.Now()), nil
}
