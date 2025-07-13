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

	"github.com/go-shiori/go-readability"

	"pre-processor/models"
	"pre-processor/retry"
	"pre-processor/utils"
)

var (
	// Domain-based rate limiter to allow concurrent requests to different domains
	domainRateLimiter = utils.NewDomainRateLimiter(5*time.Second, 1)
)

// HTTPClient interface for dependency injection.
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// DLQPublisher interface for publishing failed articles to Dead Letter Queue
type DLQPublisher interface {
	PublishFailedArticle(ctx context.Context, url string, attempts int, lastError error) error
}

// ArticleFetcherService implementation.
type articleFetcherService struct {
	logger       *slog.Logger
	httpClient   HTTPClient
	retrier      *retry.Retrier
	dlqPublisher DLQPublisher
}

// NewArticleFetcherService creates a new article fetcher service.
func NewArticleFetcherService(logger *slog.Logger) ArticleFetcherService {
	return &articleFetcherService{
		logger:     logger,
		httpClient: nil, // Will use shared HTTP client when nil
	}
}

// NewArticleFetcherServiceWithClient creates a new article fetcher service with custom HTTP client.
func NewArticleFetcherServiceWithClient(logger *slog.Logger, httpClient HTTPClient) ArticleFetcherService {
	return &articleFetcherService{
		logger:     logger,
		httpClient: httpClient,
	}
}

// NewArticleFetcherServiceWithRetryAndDLQ creates a new article fetcher service with retry and DLQ support.
func NewArticleFetcherServiceWithRetryAndDLQ(logger *slog.Logger, retrier *retry.Retrier, dlqPublisher DLQPublisher) ArticleFetcherService {
	// デフォルトのリトライ設定
	if retrier == nil {
		retryConfig := retry.RetryConfig{
			MaxAttempts:   3,
			BaseDelay:     1 * time.Second,
			MaxDelay:      30 * time.Second,
			BackoffFactor: 2.0,
			JitterFactor:  0.1,
		}
		retrier = retry.NewRetrier(retryConfig, IsRetryableError, logger)
	}
	
	return &articleFetcherService{
		logger:       logger,
		httpClient:   nil, // Will use shared HTTP client when nil
		retrier:      retrier,
		dlqPublisher: dlqPublisher,
	}
}

// FetchArticle fetches an article from the given URL.
func (s *articleFetcherService) FetchArticle(ctx context.Context, urlStr string) (*models.Article, error) {
	start := time.Now()
	s.logger.Info("Fetching article", "url", urlStr)

	// Validate URL first
	if err := s.ValidateURL(urlStr); err != nil {
		s.logger.Error("Failed to validate URL", "url", urlStr, "error", err)
		return nil, err
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		s.logger.Error("Failed to parse URL", "url", urlStr, "error", err)
		return nil, err
	}

	// Use retry mechanism if available
	if s.retrier != nil && s.dlqPublisher != nil {
		return s.fetchWithRetryAndDLQ(ctx, *parsedURL, start)
	}

	// Fallback to original implementation
	article, err := s.fetchArticleFromURL(*parsedURL)
	if err != nil {
		s.logger.Error("Failed to fetch article", "url", urlStr, "error", err)
		return nil, err
	}

	s.logger.Info("Article fetched successfully", "url", urlStr)
	return article, nil
}

// fetchWithRetryAndDLQ implements the TASK2 retry and DLQ integration
func (s *articleFetcherService) fetchWithRetryAndDLQ(ctx context.Context, parsedURL url.URL, start time.Time) (*models.Article, error) {
	urlStr := parsedURL.String()
	domain := parsedURL.Hostname()
	
	s.logger.Info("article fetch pipeline started with retry and DLQ", 
		"url", urlStr,
		"domain", domain,
		"timestamp", start.Format(time.RFC3339Nano))

	var article *models.Article
	var attemptCount int
	var totalRateLimitWait time.Duration

	operation := func() error {
		attemptCount++
		attemptStart := time.Now()
		
		s.logger.Debug("fetch attempt starting",
			"url", urlStr,
			"attempt", attemptCount,
			"attempt_start", attemptStart.Format(time.RFC3339Nano))

		// レート制限適用
		rateLimitStart := time.Now()
		domainRateLimiter.Wait(domain)
		rateLimitDuration := time.Since(rateLimitStart)
		totalRateLimitWait += rateLimitDuration

		if rateLimitDuration > 0 {
			s.logger.Debug("rate limit applied",
				"url", urlStr,
				"attempt", attemptCount,
				"wait_duration_ms", rateLimitDuration.Milliseconds())
		}

		// HTTP取得
		fetchedArticle, err := s.fetchArticleFromURL(parsedURL)
		attemptDuration := time.Since(attemptStart)

		if err != nil {
			s.logger.Warn("fetch attempt failed",
				"url", urlStr,
				"domain", domain,
				"attempt", attemptCount,
				"error", err,
				"attempt_duration_ms", attemptDuration.Milliseconds(),
				"rate_limit_wait_ms", rateLimitDuration.Milliseconds())
			return err
		}

		s.logger.Debug("fetch attempt succeeded",
			"url", urlStr,
			"domain", domain,
			"attempt", attemptCount,
			"content_size", len(fetchedArticle.Content),
			"attempt_duration_ms", attemptDuration.Milliseconds(),
			"rate_limit_wait_ms", rateLimitDuration.Milliseconds())

		article = fetchedArticle
		return nil
	}

	retryStart := time.Now()
	err := s.retrier.Do(ctx, operation)
	retryDuration := time.Since(retryStart)
	totalDuration := time.Since(start)

	if err != nil {
		s.logger.Error("article fetch failed after all retries",
			"url", urlStr,
			"domain", domain,
			"attempts", attemptCount,
			"error", err,
			"retry_duration_ms", retryDuration.Milliseconds(),
			"total_duration_ms", totalDuration.Milliseconds(),
			"total_rate_limit_wait_ms", totalRateLimitWait.Milliseconds())

		// DLQに送信（パフォーマンス測定付き）
		dlqStart := time.Now()
		if dlqErr := s.dlqPublisher.PublishFailedArticle(ctx, urlStr, attemptCount, err); dlqErr != nil {
			dlqDuration := time.Since(dlqStart)
			s.logger.Error("failed to publish to DLQ",
				"url", urlStr,
				"domain", domain,
				"dlq_error", dlqErr,
				"original_error", err,
				"dlq_duration_ms", dlqDuration.Milliseconds())
		} else {
			dlqDuration := time.Since(dlqStart)
			s.logger.Info("published failed article to DLQ",
				"url", urlStr,
				"domain", domain,
				"attempts", attemptCount,
				"dlq_duration_ms", dlqDuration.Milliseconds())
		}

		return nil, fmt.Errorf("article fetch failed: %w", err)
	}

	// 成功時の包括的パフォーマンスログ
	s.logger.Info("article fetch pipeline completed successfully",
		"url", urlStr,
		"domain", domain,
		"attempts", attemptCount,
		"content_size_bytes", len(article.Content),
		"retry_duration_ms", retryDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
		"total_rate_limit_wait_ms", totalRateLimitWait.Milliseconds(),
		"avg_attempt_duration_ms", float64(totalDuration.Milliseconds())/float64(attemptCount),
		"throughput_bytes_per_second", float64(len(article.Content))/totalDuration.Seconds())

	return article, nil
}

// ValidateURL validates a URL for security and format.
func (s *articleFetcherService) ValidateURL(urlStr string) error {
	s.logger.Info("Validating URL", "url", urlStr)

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
	start := time.Now()

	// パフォーマンスログ用の情報収集
	domain := url.Hostname()

	s.logger.Info("article fetch started",
		"url", url.String(),
		"domain", domain,
		"timestamp", start.Format(time.RFC3339Nano))

	// Enforce domain-based rate limiting
	rateLimitStart := time.Now()
	domainRateLimiter.Wait(domain)
	rateLimitDuration := time.Since(rateLimitStart)

	if rateLimitDuration > 100*time.Millisecond {
		s.logger.Info("rate limit wait applied",
			"url", url.String(),
			"domain", domain,
			"wait_duration_ms", rateLimitDuration.Milliseconds())
	}

	// Skip MP3 files
	if strings.HasSuffix(url.String(), ".mp3") {
		s.logger.Info("Skipping MP3 URL", "url", url.String())
		return nil, nil
	}

	// Validate URL for SSRF protection
	if err := s.validateURLForSSRF(&url); err != nil {
		duration := time.Since(start)
		s.logger.Error("URL validation failed",
			"url", url.String(),
			"domain", domain,
			"error", err,
			"duration_ms", duration.Milliseconds())
		return nil, err
	}

	// Use injected client or shared HTTP client manager
	var client HTTPClient
	if s.httpClient != nil {
		client = s.httpClient
	} else {
		// Use singleton HTTP client manager for better performance
		clientManager := utils.NewHTTPClientManager()
		client = &HTTPClientWrapper{clientManager.GetFeedClient()}
	}

	// HTTP リクエストの実行時間測定
	httpStart := time.Now()
	resp, err := client.Get(url.String())
	httpDuration := time.Since(httpStart)

	if err != nil {
		totalDuration := time.Since(start)
		s.logger.Error("Failed to fetch page",
			"url", url.String(),
			"domain", domain,
			"error", err,
			"http_duration_ms", httpDuration.Milliseconds(),
			"total_duration_ms", totalDuration.Milliseconds())
		return nil, err
	}
	defer resp.Body.Close()

	// レスポンス読み取り時間測定
	readStart := time.Now()
	article, err := readability.FromReader(resp.Body, &url)
	readDuration := time.Since(readStart)

	if err != nil {
		totalDuration := time.Since(start)
		s.logger.Error("Failed to parse article",
			"url", url.String(),
			"domain", domain,
			"error", err,
			"http_duration_ms", httpDuration.Milliseconds(),
			"read_duration_ms", readDuration.Milliseconds(),
			"total_duration_ms", totalDuration.Milliseconds())
		return nil, err
	}

	if article.TextContent == "" {
		totalDuration := time.Since(start)
		s.logger.Error("Article content is empty",
			"url", url.String(),
			"domain", domain,
			"http_duration_ms", httpDuration.Milliseconds(),
			"read_duration_ms", readDuration.Milliseconds(),
			"total_duration_ms", totalDuration.Milliseconds())
		return nil, errors.New("article content is empty")
	}

	cleanedContent := strings.ReplaceAll(article.TextContent, "\n", " ")
	contentSize := len(cleanedContent)
	totalDuration := time.Since(start)

	// 成功時のパフォーマンスログ
	s.logger.Info("article fetch completed successfully",
		"url", url.String(),
		"domain", domain,
		"title", article.Title,
		"content_size_bytes", contentSize,
		"rate_limit_wait_ms", rateLimitDuration.Milliseconds(),
		"http_duration_ms", httpDuration.Milliseconds(),
		"read_duration_ms", readDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
		"throughput_bytes_per_second", float64(contentSize)/totalDuration.Seconds())

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

// Get implements HTTPClient interface with proper User-Agent setting.
func (w *HTTPClientWrapper) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// User-Agent設定（TASK1.md準拠）
	req.Header.Set("User-Agent", "pre-processor/1.0 (+https://alt.example.com/bot)")

	return w.Client.Do(req)
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
