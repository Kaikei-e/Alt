package search_indexer

import (
	"alt/driver/models"
	"alt/utils/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"errors"
)

func SearchArticles(query string) ([]models.SearchArticlesHit, error) {
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
		logger.Logger.Error("Failed to create request", "error", err)
		return nil, errors.New("failed to create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "alt-backend/1.0")

	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error("Failed to send request", "error", err)
		return nil, errors.New("failed to send request")
	}

	defer resp.Body.Close()

	logger.Logger.Info("Search response received", "status", resp.StatusCode, "content-type", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error("Failed to read response body", "error", err)
		return nil, errors.New("failed to read response body")
	}

	if resp.StatusCode != http.StatusOK {
		logger.Logger.Error("Search request failed", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("search request failed with status %d", resp.StatusCode)
	}

	var response models.SearchArticlesAPIResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		logger.Logger.Error("Failed to unmarshal response body", "error", err, "body_preview", string(body))
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
