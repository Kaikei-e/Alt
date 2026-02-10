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

	"rag-orchestrator/internal/domain"
)

// RerankRequest is the request payload for the rerank endpoint.
type RerankRequest struct {
	Query      string   `json:"query"`
	Candidates []string `json:"candidates"`
	Model      string   `json:"model,omitempty"`
	TopK       int      `json:"top_k,omitempty"`
}

// RerankResponseResult is a single result in the rerank response.
type RerankResponseResult struct {
	Index int     `json:"index"`
	Score float32 `json:"score"`
}

// RerankResponse is the response from the rerank endpoint.
type RerankResponse struct {
	Results          []RerankResponseResult `json:"results"`
	Model            string                 `json:"model"`
	ProcessingTimeMs *float64               `json:"processing_time_ms,omitempty"`
}

// RerankerClient implements domain.Reranker via HTTP calls to news-creator.
type RerankerClient struct {
	BaseURL string
	Model   string
	Client  *http.Client
	logger  *slog.Logger
}

// NewRerankerClient constructs a new RerankerClient.
// baseURL should be the news-creator service URL (e.g., http://news-creator:8001).
// model should be the cross-encoder model name (e.g., bge-reranker-v2-m3).
// If client is nil, a default http.Client is created with the given timeout.
func NewRerankerClient(baseURL, model string, timeout time.Duration, logger *slog.Logger, client ...*http.Client) *RerankerClient {
	var c *http.Client
	if len(client) > 0 && client[0] != nil {
		c = client[0]
	} else {
		c = &http.Client{Timeout: timeout}
	}
	return &RerankerClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Client:  c,
		logger:  logger,
	}
}

// Rerank scores candidates against the query using a cross-encoder model.
// Returns results sorted by score descending.
func (c *RerankerClient) Rerank(ctx context.Context, query string, candidates []domain.RerankCandidate) ([]domain.RerankResult, error) {
	if len(candidates) == 0 {
		return []domain.RerankResult{}, nil
	}

	startTime := time.Now()

	c.logger.Info("reranking_started",
		slog.String("query", truncateString(query, 100)),
		slog.Int("candidate_count", len(candidates)),
		slog.String("model", c.Model))

	// Extract content strings for the request
	contents := make([]string, len(candidates))
	for i, cand := range candidates {
		contents[i] = cand.Content
	}

	reqBody := RerankRequest{
		Query:      query,
		Candidates: contents,
		Model:      c.Model,
	}

	jsonPayload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rerank request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/rerank", c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		c.logger.Warn("reranking_failed",
			slog.String("error", err.Error()),
			slog.Int64("elapsed_ms", time.Since(startTime).Milliseconds()))
		return nil, fmt.Errorf("failed to call rerank endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Warn("reranking_failed",
			slog.Int("status_code", resp.StatusCode),
			slog.String("body", truncateString(string(body), 500)),
			slog.Int64("elapsed_ms", time.Since(startTime).Milliseconds()))
		return nil, fmt.Errorf("rerank endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var rerankResp RerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode rerank response: %w", err)
	}

	// Map results back to candidate IDs
	results := make([]domain.RerankResult, len(rerankResp.Results))
	for i, r := range rerankResp.Results {
		if r.Index < 0 || r.Index >= len(candidates) {
			return nil, fmt.Errorf("invalid result index %d for %d candidates", r.Index, len(candidates))
		}
		results[i] = domain.RerankResult{
			ID:    candidates[r.Index].ID,
			Score: r.Score,
		}
	}

	elapsedMs := time.Since(startTime).Milliseconds()
	c.logger.Info("reranking_completed",
		slog.Int("result_count", len(results)),
		slog.String("model", rerankResp.Model),
		slog.Int64("elapsed_ms", elapsedMs))

	return results, nil
}

// ModelName returns the model identifier for logging/debugging.
func (c *RerankerClient) ModelName() string {
	return c.Model
}
