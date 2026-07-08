package search_indexer

import (
	"alt/driver/models"
	appErrors "alt/utils/errors"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Re-export sentinel errors from utils/errors for backward compatibility within the driver.
var (
	ErrSearchServiceUnavailable = appErrors.ErrSearchServiceUnavailable
	ErrSearchTimeout            = appErrors.ErrSearchTimeout
)

// httpClient is shared across all search-indexer REST calls so connections are pooled
// instead of a new client (and its own idle-conn pool) being created per request.
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

const (
	searchIndexerHost = "search-indexer"
	searchIndexerPort = "9300"
)

// SearchArticles searches articles via the search-indexer REST API.
func SearchArticles(ctx context.Context, query string) ([]models.SearchArticlesHit, error) {
	baseURL := fmt.Sprintf("http://%s:%s", searchIndexerHost, searchIndexerPort)
	targetEndpoint, err := BuildSearchURL(baseURL, "/v1/search", query)
	if err != nil {
		return nil, err
	}
	return doSearchRequest(ctx, targetEndpoint)
}

// SearchArticlesWithUserID searches articles with user_id parameter.
func SearchArticlesWithUserID(ctx context.Context, query string, userID string) ([]models.SearchArticlesHit, error) {
	baseURL := fmt.Sprintf("http://%s:%s", searchIndexerHost, searchIndexerPort)
	targetEndpoint, err := BuildSearchURLWithUserID(baseURL, "/v1/search", query, userID)
	if err != nil {
		return nil, err
	}
	return doSearchRequest(ctx, targetEndpoint)
}

// doSearchRequest issues the shared GET/parse/decode flow used by SearchArticles and
// SearchArticlesWithUserID (the only difference between them is the query string).
func doSearchRequest(ctx context.Context, targetEndpoint string) ([]models.SearchArticlesHit, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetEndpoint, nil)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to create request", "error", err)
		return nil, errors.New("failed to create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "alt-backend/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to send request", "error", err)
		if isTimeoutError(err) {
			return nil, ErrSearchTimeout
		}
		if isConnectionError(err) {
			return nil, ErrSearchServiceUnavailable
		}
		return nil, ErrSearchServiceUnavailable
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Logger.DebugContext(ctx, "Failed to close response body", "error", closeErr)
		}
	}()

	logger.Logger.InfoContext(ctx, "Search response received", "status", resp.StatusCode, "content-type", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to read response body", "error", err)
		return nil, errors.New("failed to read response body")
	}

	if resp.StatusCode != http.StatusOK {
		logger.Logger.ErrorContext(ctx, "Search request failed", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("search request failed with status %d", resp.StatusCode)
	}

	var response models.SearchArticlesAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to unmarshal response body", "error", err, "body_preview", string(body))
		return nil, errors.New("failed to unmarshal response body")
	}

	results := make([]models.SearchArticlesHit, 0, len(response.Hits))
	for _, hit := range response.Hits {
		results = append(results, models.SearchArticlesHit{
			ID:      hit.ID,
			Title:   hit.Title,
			Content: hit.Content,
		})
	}

	return results, nil
}

func BuildSearchURL(baseURL, path, query string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	u.Path = path

	vals := url.Values{}
	vals.Add("q", query)
	u.RawQuery = vals.Encode()

	return u.String(), nil
}

func BuildSearchURLWithUserID(baseURL, path, query, userID string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	u.Path = path

	vals := url.Values{}
	vals.Add("q", query)
	vals.Add("user_id", userID)
	u.RawQuery = vals.Encode()

	return u.String(), nil
}

// isTimeoutError reports whether err is a timeout: either the context deadline
// was exceeded, or the underlying net.Error reports Timeout().
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isConnectionError reports whether err is a low-level connection failure
// (refused, no such host, dial failure, ...), surfaced by Go's net package as *net.OpError.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}
