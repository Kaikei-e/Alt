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
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Re-export sentinel errors from utils/errors for backward compatibility within the driver.
var (
	ErrSearchServiceUnavailable = appErrors.ErrSearchServiceUnavailable
	ErrSearchTimeout            = appErrors.ErrSearchTimeout
)

func SearchArticles(ctx context.Context, query string) ([]models.SearchArticlesHit, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	host := "search-indexer"
	port := "9300"

	baseURL := fmt.Sprintf("http://%s:%s", host, port)
	targetEndpoint, err := BuildSearchURL(baseURL, "/v1/search", query)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", targetEndpoint, nil)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to create request", "error", err)
		return nil, errors.New("failed to create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "alt-backend/1.0")

	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to send request", "error", err)
		// Check if it's a timeout error
		if isTimeoutError(err) {
			return nil, ErrSearchTimeout
		}
		// Check if it's a connection error (service unavailable)
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
	err = json.Unmarshal(body, &response)
	if err != nil {
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

// SearchArticlesWithUserID searches articles with user_id parameter
func SearchArticlesWithUserID(ctx context.Context, query string, userID string) ([]models.SearchArticlesHit, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	host := "search-indexer"
	port := "9300"

	baseURL := fmt.Sprintf("http://%s:%s", host, port)
	targetEndpoint, err := BuildSearchURLWithUserID(baseURL, "/v1/search", query, userID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", targetEndpoint, nil)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to create request", "error", err, "user_id", userID)
		return nil, errors.New("failed to create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "alt-backend/1.0")

	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to send request", "error", err, "user_id", userID)
		// Check if it's a timeout error
		if isTimeoutError(err) {
			return nil, ErrSearchTimeout
		}
		// Check if it's a connection error (service unavailable)
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

	logger.Logger.InfoContext(ctx, "Search response received", "status", resp.StatusCode, "user_id", userID)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to read response body", "error", err)
		return nil, errors.New("failed to read response body")
	}

	if resp.StatusCode != http.StatusOK {
		logger.Logger.ErrorContext(ctx, "Search request failed", "status", resp.StatusCode, "body", string(body), "user_id", userID)
		return nil, fmt.Errorf("search request failed with status %d", resp.StatusCode)
	}

	var response models.SearchArticlesAPIResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
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

// isTimeoutError checks if the error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "Timeout") ||
		strings.Contains(errStr, "deadline exceeded")
}

// isConnectionError checks if the error is a connection error
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "dial tcp") ||
		strings.Contains(errStr, "connect:") ||
		strings.Contains(errStr, "i/o timeout")
}
