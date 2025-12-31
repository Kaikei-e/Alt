package rag_augur

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ExpandQueryRequest is the request payload for the query expansion endpoint.
type ExpandQueryRequest struct {
	Query         string `json:"query"`
	JapaneseCount int    `json:"japanese_count"`
	EnglishCount  int    `json:"english_count"`
}

// ExpandQueryResponse is the response from the query expansion endpoint.
type ExpandQueryResponse struct {
	ExpandedQueries  []string `json:"expanded_queries"`
	OriginalQuery    string   `json:"original_query"`
	Model            string   `json:"model"`
	ProcessingTimeMs *float64 `json:"processing_time_ms"`
}

// QueryExpanderClient calls the news-creator /api/v1/expand-query endpoint.
type QueryExpanderClient struct {
	BaseURL string
	Client  *http.Client
	logger  *slog.Logger
}

// NewQueryExpanderClient constructs a new QueryExpanderClient.
func NewQueryExpanderClient(baseURL string, timeoutSec int, logger *slog.Logger) *QueryExpanderClient {
	return &QueryExpanderClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
		logger: logger,
	}
}

// ExpandQuery calls the news-creator expand-query endpoint.
func (c *QueryExpanderClient) ExpandQuery(ctx context.Context, query string, japaneseCount, englishCount int) ([]string, error) {
	startTime := time.Now()

	c.logger.Info("query_expansion_started",
		slog.String("query", truncateString(query, 100)),
		slog.Int("japanese_count", japaneseCount),
		slog.Int("english_count", englishCount))

	reqBody := ExpandQueryRequest{
		Query:         query,
		JapaneseCount: japaneseCount,
		EnglishCount:  englishCount,
	}

	jsonPayload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal expand query request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/expand-query", c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create expand query request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		c.logger.Warn("query_expansion_failed",
			slog.String("error", err.Error()),
			slog.Int64("elapsed_ms", time.Since(startTime).Milliseconds()))
		return nil, fmt.Errorf("failed to call expand query endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Warn("query_expansion_failed",
			slog.Int("status_code", resp.StatusCode),
			slog.String("body", truncateString(string(body), 500)),
			slog.Int64("elapsed_ms", time.Since(startTime).Milliseconds()))
		return nil, fmt.Errorf("expand query endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var expandResp ExpandQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&expandResp); err != nil {
		return nil, fmt.Errorf("failed to decode expand query response: %w", err)
	}

	elapsedMs := time.Since(startTime).Milliseconds()
	c.logger.Info("query_expansion_completed",
		slog.Int("expanded_count", len(expandResp.ExpandedQueries)),
		slog.String("model", expandResp.Model),
		slog.Int64("elapsed_ms", elapsedMs))

	return expandResp.ExpandedQueries, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
