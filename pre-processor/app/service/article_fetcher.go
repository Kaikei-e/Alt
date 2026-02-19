package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"log/slog"

	"pre-processor/config"
	"pre-processor/domain"
)

// URL scheme constants
const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
)

// HTTPClient interface for dependency injection.
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// articleFetcherService implementation.
type articleFetcherService struct {
	logger     *slog.Logger
	httpClient HTTPClient
}

// NewArticleFetcherService creates a new article fetcher service.
func NewArticleFetcherService(logger *slog.Logger) ArticleFetcherService {
	return &articleFetcherService{
		logger: logger,
	}
}

// NewArticleFetcherServiceWithClient creates a new article fetcher service with custom HTTP client.
func NewArticleFetcherServiceWithClient(logger *slog.Logger, httpClient HTTPClient) ArticleFetcherService {
	return &articleFetcherService{
		logger:     logger,
		httpClient: httpClient,
	}
}

// NewArticleFetcherServiceWithFactory creates a new article fetcher service with HTTPClientFactory.
func NewArticleFetcherServiceWithFactory(cfg *config.Config, logger *slog.Logger) ArticleFetcherService {
	factory := NewHTTPClientFactory(cfg, logger)
	httpClient := factory.CreateArticleFetcherClient()

	return &articleFetcherService{
		logger:     logger,
		httpClient: httpClient,
	}
}

// FetchArticle is disabled for ethical compliance.
func (s *articleFetcherService) FetchArticle(ctx context.Context, urlStr string) (*domain.Article, error) {
	s.logger.InfoContext(ctx, "Article fetching disabled for ethical compliance", "url", urlStr)
	return nil, nil
}

// ValidateURL validates a URL for security and format.
func (s *articleFetcherService) ValidateURL(urlStr string) error {
	if urlStr == "" {
		return errors.New("URL cannot be empty")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return err
	}

	if parsedURL.Scheme != SchemeHTTP && parsedURL.Scheme != SchemeHTTPS {
		return errors.New("only HTTP or HTTPS schemes allowed")
	}

	if err := s.validateURLForSSRF(parsedURL); err != nil {
		return err
	}

	if port := parsedURL.Port(); port != "" {
		if err := s.validateTarget(parsedURL.Hostname(), port); err != nil {
			return err
		}
	}

	if parsedURL.Hostname() == "" {
		return errors.New("URL must contain a host")
	}

	return nil
}

// HTTPClientWrapper wraps http.Client to implement HTTPClient interface.
type HTTPClientWrapper struct {
	*http.Client
	UserAgent string
	Config    *config.HTTPConfig
}

// Get implements HTTPClient interface with proper User-Agent setting.
func (w *HTTPClientWrapper) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	userAgent := w.UserAgent
	if userAgent == "" {
		if w.Config != nil {
			userAgent = w.Config.UserAgent
		} else {
			userAgent = "Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)"
		}
	}

	req.Header.Set("User-Agent", userAgent)

	if w.Config != nil && w.Config.EnableBrowserHeaders {
		headers := w.Config.GetBrowserHeaders(userAgent)
		for key, value := range headers {
			if key != "User-Agent" {
				req.Header.Set(key, value)
			}
		}
	}

	start := time.Now()
	resp, err := w.Do(req)
	duration := time.Since(start)

	metrics := GetGlobalProxyMetrics(nil)

	if err != nil {
		var errorType ProxyErrorType
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			errorType = ProxyErrorTimeout
		} else {
			errorType = ProxyErrorConnection
		}

		metrics.RecordDomainRequest(url, duration, false, errorType)
		return nil, err
	}

	if w.Config != nil && w.Config.SkipErrorResponses && resp.StatusCode >= 400 {
		errorType := ProxyErrorConnection
		metrics.RecordDomainRequest(url, duration, false, errorType)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("HTTP error response: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	if success {
		metrics.RecordDomainRequest(url, duration, true, ProxyErrorConfig)
	}

	return resp, nil
}

func (s *articleFetcherService) validateURLForSSRF(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("only HTTP or HTTPS schemes allowed")
	}

	if s.isPrivateHost(u.Hostname()) {
		return errors.New("access to private networks not allowed")
	}

	return nil
}

func (s *articleFetcherService) validateTarget(host, port string) error {
	blockedPorts := map[string]bool{
		"22": true, "23": true, "25": true, "53": true, "110": true,
		"143": true, "993": true, "995": true, "1433": true, "3306": true,
		"5432": true, "6379": true, "11211": true,
	}

	if blockedPorts[port] {
		return errors.New("access to this port is not allowed")
	}

	if s.isPrivateHost(host) {
		return errors.New("access to private networks not allowed")
	}

	return nil
}

func (s *articleFetcherService) isPrivateHost(hostname string) bool {
	ip := net.ParseIP(hostname)
	if ip != nil {
		return s.isPrivateIPAddress(ip)
	}

	hostname = strings.ToLower(hostname)
	if hostname == "localhost" || strings.HasPrefix(hostname, "127.") {
		return true
	}

	if hostname == "169.254.169.254" || hostname == "metadata.google.internal" {
		return true
	}

	internalDomains := []string{".local", ".internal", ".corp", ".lan"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostname, domain) {
			return true
		}
	}

	return false
}

func (s *articleFetcherService) isPrivateIPAddress(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
			return true
		case ip4[0] == 127:
			return true
		}
	}

	if ip6 := ip.To16(); ip6 != nil {
		if ip6[0] == 0xfe && ip6[1] == 0x80 {
			return true
		}

		if ip6[0] == 0xfc && ip6[1] == 0x00 {
			return true
		}
	}

	return false
}
