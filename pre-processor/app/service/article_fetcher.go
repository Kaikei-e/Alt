package service

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"pre-processor/models"

	"github.com/go-shiori/go-readability"
)

var (
	// Global rate limiter to ensure minimum 5 seconds between requests.
	lastRequestTime time.Time
	rateLimitMutex  sync.Mutex
	minInterval     = 5 * time.Second
)

// HTTPClient interface for dependency injection.
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// ArticleFetcherService implementation.
type articleFetcherService struct {
	logger     *slog.Logger
	httpClient HTTPClient
}

// NewArticleFetcherService creates a new article fetcher service.
func NewArticleFetcherService(logger *slog.Logger) ArticleFetcherService {
	return &articleFetcherService{
		logger:     logger,
		httpClient: nil, // Will use createSecureHTTPClient() when nil
	}
}

// NewArticleFetcherServiceWithClient creates a new article fetcher service with custom HTTP client.
func NewArticleFetcherServiceWithClient(logger *slog.Logger, httpClient HTTPClient) ArticleFetcherService {
	return &articleFetcherService{
		logger:     logger,
		httpClient: httpClient,
	}
}

// FetchArticle fetches an article from the given URL.
func (s *articleFetcherService) FetchArticle(ctx context.Context, urlStr string) (*models.Article, error) {
	s.logger.Info("fetching article", "url", urlStr)

	// Validate URL first
	if err := s.ValidateURL(urlStr); err != nil {
		s.logger.Error("failed to validate URL", "url", urlStr, "error", err)
		return nil, err
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		s.logger.Error("failed to parse URL", "url", urlStr, "error", err)
		return nil, err
	}

	// Initialize global logger if needed
	if slog.Default() == nil {
		// logger initialization no longer needed
	}

	// Fetch article using embedded logic
	article, err := s.fetchArticleFromURL(*parsedURL)
	if err != nil {
		s.logger.Error("failed to fetch article", "url", urlStr, "error", err)
		return nil, err
	}

	s.logger.Info("article fetched successfully", "url", urlStr)

	return article, nil
}

// ValidateURL validates a URL for security and format.
func (s *articleFetcherService) ValidateURL(urlStr string) error {
	s.logger.Info("validating URL", "url", urlStr)

	// Validate empty string
	if urlStr == "" {
		s.logger.Error("URL validation failed", "url", urlStr, "error", "URL cannot be empty")
		return errors.New("URL cannot be empty")
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		s.logger.Error("URL validation failed", "url", urlStr, "error", err)
		return err
	}

	// Validate scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		s.logger.Error("URL validation failed", "url", urlStr, "error", "only HTTP or HTTPS schemes allowed")
		return errors.New("only HTTP or HTTPS schemes allowed")
	}

	// Additional SSRF and port validation
	if err := s.validateURLForSSRF(parsedURL); err != nil {
		s.logger.Error("URL validation failed", "url", urlStr, "error", err)
		return err
	}

	if port := parsedURL.Port(); port != "" {
		if err := s.validateTarget(parsedURL.Hostname(), port); err != nil {
			s.logger.Error("URL validation failed", "url", urlStr, "error", err)
			return err
		}
	}

	// Validate host
	if parsedURL.Hostname() == "" {
		s.logger.Error("URL validation failed", "url", urlStr, "error", "missing host")
		return errors.New("URL must contain a host")
	}

	s.logger.Info("URL validated successfully", "url", urlStr)

	return nil
}

// fetchArticleFromURL fetches an article from a URL (moved from article-fetcher package).
func (s *articleFetcherService) fetchArticleFromURL(url url.URL) (*models.Article, error) {
	// Enforce rate limiting
	rateLimitMutex.Lock()
	if !lastRequestTime.IsZero() {
		elapsed := time.Since(lastRequestTime)
		if elapsed < minInterval {
			waitTime := minInterval - elapsed
			s.logger.Info("Rate limiting: waiting before next request",
				"wait_time", waitTime,
				"url", url.String())
			time.Sleep(waitTime)
		}
	}

	lastRequestTime = time.Now()
	rateLimitMutex.Unlock()

	// Skip MP3 files
	if strings.HasSuffix(url.String(), ".mp3") {
		s.logger.Info("Skipping MP3 URL", "url", url.String())
		return nil, nil
	}

	// Validate URL for SSRF protection
	if err := s.validateURLForSSRF(&url); err != nil {
		s.logger.Error("URL validation failed", "error", err, "url", url.String())
		return nil, err
	}

	// Use injected client or create secure HTTP client
	var client HTTPClient
	if s.httpClient != nil {
		client = s.httpClient
	} else {
		client = s.createSecureHTTPClient()
	}

	// Fetch the page
	resp, err := client.Get(url.String())
	if err != nil {
		s.logger.Error("Failed to fetch page", "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse with readability
	article, err := readability.FromReader(resp.Body, &url)
	if err != nil {
		s.logger.Error("Failed to parse article", "error", err)
		return nil, err
	}

	if article.TextContent == "" {
		s.logger.Error("Article content is empty", "url", url.String())
		return nil, errors.New("article content is empty")
	}

	s.logger.Info("Article fetched", "title", article.Title, "content length", len(article.TextContent))

	cleanedContent := strings.ReplaceAll(article.TextContent, "\n", " ")

	return &models.Article{
		Title:   article.Title,
		Content: cleanedContent,
		URL:     url.String(),
	}, nil
}

// HTTPClientWrapper wraps http.Client to implement HTTPClient interface.
type HTTPClientWrapper struct {
	*http.Client
}

// Get implements HTTPClient interface.
func (w *HTTPClientWrapper) Get(url string) (*http.Response, error) {
	return w.Client.Get(url)
}

// Helper methods (moved from article-fetcher package).
func (s *articleFetcherService) createSecureHTTPClient() HTTPClient {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			if err := s.validateTarget(host, port); err != nil {
				return nil, err
			}

			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
	}

	return &HTTPClientWrapper{
		Client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
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
