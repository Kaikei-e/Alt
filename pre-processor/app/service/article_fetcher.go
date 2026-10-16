package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
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
	Get(ctx context.Context, url string) (*http.Response, error)
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
	return nil, domain.ErrFetchDisabled
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
func (w *HTTPClientWrapper) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
		errorType := classifyProxyError(err)
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

// isPrivateIPAddress reports whether ip must not be reached from the
// fetcher (SSRF guard).
func (s *articleFetcherService) isPrivateIPAddress(ip net.IP) bool {
	return isPrivateOrBlocked(ip)
}

// isPrivateOrBlocked normalizes IPv4-mapped IPv6 addresses via netip.Addr.Unmap
// and blocks loopback, RFC 1918 private, RFC 6598 CGNAT (100.64.0.0/10),
// link-local/cloud metadata (169.254.0.0/16, fe80::/10, 192.0.0.0/24),
// benchmarking (198.18.0.0/15), multicast, broadcast/reserved (240.0.0.0/4),
// and transition/deprecated IPv6 prefixes (::/96, 64:ff9b::/96, 2002::/16, fec0::/10, 100::/64).
func isPrivateOrBlocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()

	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return true
	}

	if addr.Is4() {
		b := addr.As4()
		// 0.0.0.0/8 ("this network", RFC 1122)
		if b[0] == 0 {
			return true
		}
		// 100.64.0.0/10 (RFC 6598 CGNAT, includes Alibaba Cloud metadata 100.100.100.200)
		if b[0] == 100 && b[1] >= 64 && b[1] <= 127 {
			return true
		}
		// 169.254.0.0/16 (RFC 3927 link-local / AWS, GCP, Azure metadata 169.254.169.254)
		if b[0] == 169 && b[1] == 254 {
			return true
		}
		// 192.0.0.0/24 (RFC 6890 / Oracle Cloud metadata 192.0.0.192)
		if b[0] == 192 && b[1] == 0 && b[2] == 0 {
			return true
		}
		// 198.18.0.0/15 (RFC 2544 benchmarking)
		if b[0] == 198 && (b[1] == 18 || b[1] == 19) {
			return true
		}
		// 240.0.0.0/4 (RFC 1112 reserved / broadcast 255.255.255.255)
		if b[0] >= 240 {
			return true
		}
		return false
	}

	if addr.Is6() {
		b := addr.As16()
		// IPv4-compatible IPv6 (::/96, RFC 4291 section 2.5.5.1)
		if isAllZeroBytes(b[0:12]) {
			return isPrivateOrBlocked(net.IPv4(b[12], b[13], b[14], b[15]))
		}
		// NAT64 well-known prefix (64:ff9b::/96, RFC 6052)
		if b[0] == 0x00 && b[1] == 0x64 && b[2] == 0xff && b[3] == 0x9b && isAllZeroBytes(b[4:12]) {
			return true
		}
		// 6to4 prefix (2002::/16, RFC 3056)
		if b[0] == 0x20 && b[1] == 0x02 {
			return true
		}
		// Site-local unicast (fec0::/10, RFC 3879)
		if b[0] == 0xfe && (b[1]&0xc0) == 0xc0 {
			return true
		}
		// Discard-only prefix (100::/64, RFC 6666)
		if b[0] == 0x01 && b[1] == 0x00 && isAllZeroBytes(b[2:8]) {
			return true
		}
	}

	return false
}

func isAllZeroBytes(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
