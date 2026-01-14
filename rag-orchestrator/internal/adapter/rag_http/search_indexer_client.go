package rag_http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"rag-orchestrator/internal/domain"
)

type SearchIndexerClient struct {
	BaseURL string
	Client  *http.Client
}

func NewSearchIndexerClient(baseURL string, timeout int) *SearchIndexerClient {
	return &SearchIndexerClient{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

type searchArticlesResponse struct {
	Query string  `json:"query"`
	Hits  []dhHit `json:"hits"`
}

type dhHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

func (c *SearchIndexerClient) Search(ctx context.Context, query string) ([]domain.SearchHit, error) {
	systemUserID := "rag-orchestrator-system"

	u, err := url.Parse(fmt.Sprintf("%s/v1/search", c.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	q := u.Query()
	q.Set("q", query)
	q.Set("user_id", systemUserID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status: %d", resp.StatusCode)
	}

	var sResp searchArticlesResponse
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	hits := make([]domain.SearchHit, len(sResp.Hits))
	for i, h := range sResp.Hits {
		hits[i] = domain.SearchHit{
			ID:      h.ID,
			Title:   h.Title,
			Content: h.Content,
			Tags:    h.Tags,
		}
	}

	return hits, nil
}

// SearchBM25 performs BM25 (keyword) search for hybrid search fusion.
// Implements domain.BM25Searcher interface.
func (c *SearchIndexerClient) SearchBM25(ctx context.Context, query string, limit int) ([]domain.BM25SearchResult, error) {
	systemUserID := "rag-orchestrator-system"

	u, err := url.Parse(fmt.Sprintf("%s/v1/search", c.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	q := u.Query()
	q.Set("q", query)
	q.Set("user_id", systemUserID)
	q.Set("limit", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bm25 search request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bm25 search returned status: %d", resp.StatusCode)
	}

	var sResp searchArticlesResponse
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return nil, fmt.Errorf("failed to decode bm25 search response: %w", err)
	}

	results := make([]domain.BM25SearchResult, len(sResp.Hits))
	for i, h := range sResp.Hits {
		results[i] = domain.BM25SearchResult{
			ArticleID: h.ID,
			ChunkID:   "", // Meilisearch returns article-level, not chunk-level
			Content:   h.Content,
			Title:     h.Title,
			URL:       "", // Not available from current response
			Rank:      i + 1,
			Score:     0, // BM25 score not exposed by search-indexer currently
		}
	}

	return results, nil
}
