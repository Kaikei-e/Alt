package image_fetch_gateway

import (
	"alt/domain"
	"alt/utils/errors"
	"alt/utils/logger"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProxyMode represents different proxy operation modes
type ProxyMode string

const (
	ProxyModeSidecar  ProxyMode = "sidecar"
	ProxyModeEnvoy    ProxyMode = "envoy"
	ProxyModeNginx    ProxyMode = "nginx"
	ProxyModeDisabled ProxyMode = "disabled"
)

// ProxyStrategy represents the proxy configuration strategy
type ProxyStrategy struct {
	Mode         ProxyMode
	BaseURL      string
	PathTemplate string
	Enabled      bool
}

// getProxyStrategy determines the appropriate proxy strategy based on environment configuration
func getProxyStrategy() *ProxyStrategy {
	// Priority order: SIDECAR > ENVOY > NGINX > DISABLED
	if os.Getenv("SIDECAR_PROXY_ENABLED") == "true" {
		baseURL := os.Getenv("SIDECAR_PROXY_URL")
		if baseURL == "" {
			baseURL = "http://envoy-proxy.alt-apps.svc.cluster.local:8085"
		}
		logger.SafeInfo("Image proxy strategy: SIDECAR mode selected",
			"base_url", baseURL,
			"path_template", "/proxy/{scheme}://{host}{path}")
		return &ProxyStrategy{
			Mode:         ProxyModeSidecar,
			BaseURL:      baseURL,
			PathTemplate: "/proxy/{scheme}://{host}{path}",
			Enabled:      true,
		}
	}

	if os.Getenv("ENVOY_PROXY_ENABLED") == "true" {
		baseURL := os.Getenv("ENVOY_PROXY_URL")
		if baseURL == "" {
			baseURL = "http://envoy-proxy.alt-apps.svc.cluster.local:8080"
		}
		logger.SafeInfo("Image proxy strategy: ENVOY mode selected",
			"base_url", baseURL,
			"path_template", "/proxy/{scheme}://{host}{path}")
		return &ProxyStrategy{
			Mode:         ProxyModeEnvoy,
			BaseURL:      baseURL,
			PathTemplate: "/proxy/{scheme}://{host}{path}",
			Enabled:      true,
		}
	}

	logger.SafeInfo("Image proxy strategy: DISABLED mode - direct connection will be used")
	return &ProxyStrategy{
		Mode:         ProxyModeDisabled,
		BaseURL:      "",
		PathTemplate: "",
		Enabled:      false,
	}
}

// EnvoyProxyRoundTripper fixes Host header for Envoy Dynamic Forward Proxy
type EnvoyProxyRoundTripper struct {
	transport http.RoundTripper
}

func (ert *EnvoyProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if this is an Envoy proxy request (/proxy/https://domain.com/path)
	if strings.Contains(req.URL.Path, "/proxy/https://") || strings.Contains(req.URL.Path, "/proxy/http://") {
		// Extract target domain from proxy path
		// /proxy/https://example.com/image.jpg -> example.com
		pathParts := strings.SplitN(req.URL.Path, "/proxy/", 2)
		if len(pathParts) == 2 {
			targetURL := pathParts[1]
			if parsedTarget, err := url.Parse(targetURL); err == nil {
				// Set Host header to target domain for proper TLS SNI
				req.Host = parsedTarget.Host
				req.Header.Set("Host", parsedTarget.Host)
				// CRITICAL FIX: Add X-Target-Domain header required by Envoy proxy route matching
				req.Header.Set("X-Target-Domain", parsedTarget.Host)
				logger.SafeInfo("Fixed Host header for Envoy Dynamic Forward Proxy (image fetch)",
					"original_host", req.URL.Host,
					"target_host", parsedTarget.Host,
					"request_url", req.URL.String())
			}
		}
	}
	return ert.transport.RoundTrip(req)
}

// convertToProxyURL converts external image URLs to appropriate proxy routes based on strategy
func convertToProxyURL(originalURL string, strategy *ProxyStrategy) string {
	if strategy == nil || !strategy.Enabled {
		return originalURL
	}

	// Parse original URL using net/url to prevent injection attacks
	u, err := url.Parse(originalURL)
	if err != nil {
		logger.SafeError("Failed to parse original URL for proxy conversion (image fetch)",
			"url", originalURL,
			"strategy_mode", string(strategy.Mode),
			"error", err.Error())
		return originalURL
	}

	// Validate URL components to prevent malicious inputs
	if u.Scheme == "" || u.Host == "" {
		logger.SafeError("Invalid URL components detected (image fetch)",
			"url", originalURL,
			"scheme", u.Scheme,
			"host", u.Host)
		return originalURL
	}

	// Parse base URL for proxy strategy
	baseURL, err := url.Parse(strategy.BaseURL)
	if err != nil {
		logger.SafeError("Failed to parse base URL for proxy strategy (image fetch)",
			"base_url", strategy.BaseURL,
			"error", err.Error())
		return originalURL
	}

	// Construct target URL components safely
	// Format: /proxy/https://domain.com/image.jpg
	targetURLStr := u.Scheme + "://" + u.Host + u.Path
	if u.RawQuery != "" {
		targetURLStr += "?" + u.RawQuery
	}

	// Manual path construction with security validation
	proxyPath := "/proxy/" + targetURLStr

	// Parse the complete proxy URL to ensure proper validation
	proxyURL, err := url.Parse(baseURL.String() + proxyPath)
	if err != nil {
		logger.SafeError("Failed to parse constructed proxy URL (image fetch)",
			"base_url", strategy.BaseURL,
			"proxy_path", proxyPath,
			"error", err.Error())
		return originalURL
	}

	logger.SafeInfo("Image URL converted using secure proxy strategy",
		"strategy_mode", string(strategy.Mode),
		"original_url", originalURL,
		"proxy_url", proxyURL.String(),
		"target_host", u.Host,
		"base_url", strategy.BaseURL)

	return proxyURL.String()
}

// ImageFetchGateway implements the ImageFetchPort interface
// It acts as an Anti-Corruption Layer between the domain and external HTTP APIs
type ImageFetchGateway struct {
	httpClient    *http.Client
	proxyStrategy *ProxyStrategy
}

// NewImageFetchGateway creates a new ImageFetchGateway
func NewImageFetchGateway(httpClient *http.Client) *ImageFetchGateway {
	strategy := getProxyStrategy()
	
	// If we have a proxy strategy, wrap the client with proxy-aware transport
	if strategy != nil && strategy.Enabled && httpClient != nil {
		// Get the existing transport or create a new one
		transport := httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		
		// Wrap with Envoy proxy round tripper for Host header fixing
		httpClient.Transport = &EnvoyProxyRoundTripper{
			transport: transport,
		}
	}
	
	return &ImageFetchGateway{
		httpClient:    httpClient,
		proxyStrategy: strategy,
	}
}

// FetchImage fetches an image from external URL through HTTP client
func (g *ImageFetchGateway) FetchImage(ctx context.Context, imageURL *url.URL, options *domain.ImageFetchOptions) (*domain.ImageFetchResult, error) {
	// Check context cancellation early
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Convert to proxy URL if proxy strategy is enabled
	requestURL := imageURL.String()
	if g.proxyStrategy != nil && g.proxyStrategy.Enabled {
		requestURL = convertToProxyURL(imageURL.String(), g.proxyStrategy)
		logger.SafeInfo("Using proxy strategy for image fetch",
			"strategy_mode", string(g.proxyStrategy.Mode),
			"original_url", imageURL.String(),
			"proxy_url", requestURL)
	} else {
		logger.SafeInfo("Using direct connection for image fetch (no proxy configured)",
			"original_url", imageURL.String())
	}

	// Create HTTP request with proper headers
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
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
	req.Header.Set("User-Agent", "ALT-RSS-Reader/1.0 (+https://alt.example.com)")
	req.Header.Set("Accept", "image/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Cache-Control", "no-cache")

	// Make the HTTP request
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
	defer resp.Body.Close()

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
			if int(contentLength) > options.MaxSize {
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