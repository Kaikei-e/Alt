package summarization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"alt/di"
	"alt/utils/errors"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"alt/utils/security"

	"github.com/labstack/echo/v4"
)

// ensureArticleRecord fetches or persists an article for the provided feed URL.
func ensureArticleRecord(ctx context.Context, container *di.ApplicationComponents, feedURL string) (string, string, bool, error) {
	existingArticle, err := container.AltDBRepository.FetchArticleByURL(ctx, feedURL)
	if err != nil {
		return "", "", false, err
	}
	if existingArticle != nil {
		return existingArticle.ID, existingArticle.Title, true, nil
	}

	fetchedContent, _, fetchedTitle, err := fetchArticleContent(ctx, feedURL, container)
	if err != nil {
		return "", "", false, err
	}

	articleID, err := container.AltDBRepository.SaveArticle(ctx, feedURL, fetchedTitle, fetchedContent)
	if err != nil {
		return "", "", false, err
	}

	return articleID, fetchedTitle, false, nil
}

func respondWithSummary(c echo.Context, summary, articleID, feedURL string) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":    true,
		"summary":    summary,
		"article_id": articleID,
		"feed_url":   feedURL,
	})
}

func callPreProcessorSummarize(ctx context.Context, content, articleID, title, preProcessorURL string) (string, error) {
	if articleID == "" {
		return "", fmt.Errorf("article_id is required")
	}

	requestBody := map[string]string{
		"content":    content,
		"article_id": articleID,
		"title":      title,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{
		Timeout: 300 * time.Second,
	}

	apiURL := fmt.Sprintf("%s/api/v1/summarize", preProcessorURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call pre-processor: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pre-processor returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Success   bool   `json:"success"`
		Summary   string `json:"summary"`
		ArticleID string `json:"article_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !response.Success {
		return "", fmt.Errorf("summarization failed")
	}

	return response.Summary, nil
}

func streamPreProcessorSummarize(ctx context.Context, content, articleID, title, preProcessorURL string) (io.ReadCloser, error) {
	if articleID == "" {
		return nil, fmt.Errorf("article_id is required")
	}

	requestBody := map[string]string{
		"content":    content,
		"article_id": articleID,
		"title":      title,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{
		Timeout: 0,
	}

	apiURL := fmt.Sprintf("%s/api/v1/summarize/stream", preProcessorURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call pre-processor stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - error response has been read
			_ = closeErr
		}

		errorBody := string(bodyBytes)
		if readErr != nil {
			errorBody = fmt.Sprintf("(failed to read error body: %v)", readErr)
		}

		logger.Logger.ErrorContext(ctx, "Pre-processor stream returned non-200 status",
			"status_code", resp.StatusCode,
			"status", resp.Status,
			"error_body", errorBody,
			"article_id", articleID)
		return nil, fmt.Errorf("pre-processor stream returned status %d: %s", resp.StatusCode, errorBody)
	}

	logger.Logger.InfoContext(ctx, "Pre-processor stream response received successfully",
		"article_id", articleID,
		"status", resp.Status,
		"content_type", resp.Header.Get("Content-Type"))

	return resp.Body, nil
}

func callPreProcessorSummarizeQueue(ctx context.Context, articleID, title, preProcessorURL string) (string, error) {
	if articleID == "" {
		return "", fmt.Errorf("article_id is required")
	}

	requestBody := map[string]string{
		"article_id": articleID,
		"title":      title,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	apiURL := fmt.Sprintf("%s/api/v1/summarize/queue", preProcessorURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to submit queue request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("queue endpoint returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Success bool   `json:"success"`
		JobID   string `json:"job_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to parse queue response: %w", err)
	}

	if !response.Success {
		return "", fmt.Errorf("queue request failed")
	}

	return response.JobID, nil
}

type SummarizeStatus struct {
	JobID        string `json:"job_id"`
	Status       string `json:"status"`
	Summary      string `json:"summary"`
	ErrorMessage string `json:"error_message"`
	ArticleID    string `json:"article_id"`
}

func callPreProcessorSummarizeStatus(ctx context.Context, jobID, preProcessorURL string) (*SummarizeStatus, error) {
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	apiURL := fmt.Sprintf("%s/api/v1/summarize/status/%s", preProcessorURL, jobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check job status: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status endpoint returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var status SummarizeStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to parse status response: %w", err)
	}

	return &status, nil
}

func handleError(c echo.Context, err error, operation string) error {
	ctx := c.Request().Context()
	var enrichedErr *errors.AppContextError

	if appContextErr, ok := err.(*errors.AppContextError); ok {
		enrichedErr = errors.EnrichWithContext(appContextErr, "summarization", "RESTHandler", operation, map[string]interface{}{
			"path":        c.Request().URL.Path,
			"method":      c.Request().Method,
			"remote_addr": c.Request().RemoteAddr,
			"user_agent":  c.Request().UserAgent(),
			"request_id":  c.Response().Header().Get("X-Request-ID"),
		})
	} else if appErr, ok := err.(*errors.AppError); ok {
		enrichedErr = errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			"summarization",
			"RESTHandler",
			operation,
			appErr.Cause,
			map[string]interface{}{
				"path":           c.Request().URL.Path,
				"method":         c.Request().Method,
				"remote_addr":    c.Request().RemoteAddr,
				"user_agent":     c.Request().UserAgent(),
				"request_id":     c.Response().Header().Get("X-Request-ID"),
				"legacy_context": appErr.Context,
			},
		)
	} else {
		enrichedErr = errors.NewUnknownContextError(
			"internal server error",
			"summarization",
			"RESTHandler",
			operation,
			err,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	}

	logger.Logger.ErrorContext(ctx, "REST API Error", "error", enrichedErr.Error(), "code", enrichedErr.Code, "operation", operation, "path", c.Request().URL.Path)

	return c.JSON(enrichedErr.HTTPStatusCode(), map[string]interface{}{
		"error": map[string]interface{}{
			"code":    enrichedErr.Code,
			"message": enrichedErr.Message,
		},
	})
}

func handleValidationError(c echo.Context, message, field string, value interface{}) error {
	ctx := c.Request().Context()
	logger.Logger.WarnContext(ctx, "Validation error", "message", message, "field", field, "value", value)
	return c.JSON(http.StatusBadRequest, map[string]interface{}{
		"error": message,
		"field": field,
		"value": value,
		"code":  "VALIDATION_ERROR",
	})
}

func fetchArticleContent(ctx context.Context, urlStr string, container *di.ApplicationComponents) (string, string, string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid url: %w", err)
	}

	if err := isAllowedURL(parsedURL); err != nil {
		return "", "", "", fmt.Errorf("url not allowed: %w", err)
	}

	ssrfValidator := security.NewSSRFValidator()
	if err := ssrfValidator.ValidateURL(ctx, parsedURL); err != nil {
		return "", "", "", fmt.Errorf("ssrf validation failed: %w", err)
	}

	secureClient := ssrfValidator.CreateSecureHTTPClient(10 * time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AltBot/1.0; +http://alt.com/bot)")

	// SSRF protection: URL validated by isAllowedURL() (line 360) and SSRFValidator.ValidateURL() (line 365).
	// secureClient created via SSRFValidator.CreateSecureHTTPClient() validates IPs at connection time.
	// codeql[go/request-forgery]
	resp, err := secureClient.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read body: %w", err)
	}

	htmlContent := string(bodyBytes)
	htmlLength := len(htmlContent)

	title := html_parser.ExtractTitle(htmlContent)
	extractedText := html_parser.ExtractArticleText(htmlContent)
	extractedLength := len(extractedText)
	reductionRatio := (1.0 - float64(extractedLength)/float64(htmlLength)) * 100.0

	logger.Logger.InfoContext(ctx, "Text extraction completed in fetchArticleContent",
		"url", urlStr,
		"html_length", htmlLength,
		"extracted_length", extractedLength,
		"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))

	if extractedText == "" {
		logger.Logger.WarnContext(ctx, "failed to extract article text from HTML, falling back to raw HTML",
			"url", urlStr,
			"html_size_bytes", len(htmlContent))
		return htmlContent, generateArticleID(urlStr), title, nil
	}

	originalSize := len(htmlContent)
	extractedSize := len(extractedText)
	if originalSize > 0 {
		reductionRatio := float64(extractedSize) / float64(originalSize) * 100
		logger.Logger.InfoContext(ctx, "text extraction completed",
			"url", urlStr,
			"original_size_bytes", originalSize,
			"extracted_size_bytes", extractedSize,
			"reduction_ratio_percent", reductionRatio)
	}

	articleID := generateArticleID(urlStr)
	return extractedText, articleID, title, nil
}

func generateArticleID(feedURL string) string {
	return fmt.Sprintf("article_%s", strings.ReplaceAll(feedURL, "/", "_"))
}

func isAllowedURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme not allowed: %s", u.Scheme)
	}

	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return fmt.Errorf("could not resolve hostname: %w", err)
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() {
			return fmt.Errorf("private IP not allowed: %s", ip.String())
		}
	}
	return nil
}
