package article_content_extractor_gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"alt/orchestrator/port/article_content_extractor_port"
	"alt/utils/html_parser"
	"alt/utils/security"
)

// Gateway fetches and extracts article content from external URLs.
type Gateway struct {
	logger        *slog.Logger
	ssrfValidator *security.SSRFValidator
}

// New creates a new article content extractor gateway.
func New(logger *slog.Logger) *Gateway {
	if logger == nil {
		panic("article_content_extractor_gateway: logger is required (see .claude/rules/di-wiring.md)")
	}
	return &Gateway{
		logger:        logger,
		ssrfValidator: security.NewSSRFValidator(),
	}
}

// SetTestingMode enables testing mode on the SSRF validator for unit tests.
func (g *Gateway) SetTestingMode(enabled bool) {
	g.ssrfValidator.SetTestingMode(enabled)
}

var _ article_content_extractor_port.ArticleContentExtractorPort = (*Gateway)(nil)

// ExtractArticleContent fetches and extracts content from a URL with SSRF protection.
func (g *Gateway) ExtractArticleContent(ctx context.Context, urlStr string) (string, string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	if err := security.NewURLSecurityValidator().ValidateParsedRSSURL(parsedURL); err != nil {
		return "", "", fmt.Errorf("URL not allowed: %w", err)
	}

	if err := g.ssrfValidator.ValidateURL(ctx, parsedURL); err != nil {
		return "", "", fmt.Errorf("ssrf validation failed: %w", err)
	}

	secureClient := g.ssrfValidator.CreateSecureHTTPClient(10 * time.Second)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AltBot/1.0; +http://alt.com/bot)")

	// SSRF: secureClient performs connection-time IP validation to prevent DNS rebinding.
	resp, err := secureClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", "", fmt.Errorf("failed to read body: %w", err)
	}

	htmlContent := string(bodyBytes)
	title := html_parser.ExtractTitle(htmlContent)
	extractedText := html_parser.ExtractArticleText(htmlContent)

	if extractedText == "" {
		g.logger.WarnContext(ctx, "failed to extract article text, using raw HTML", "url", urlStr)
		return htmlContent, title, nil
	}

	return extractedText, title, nil
}
