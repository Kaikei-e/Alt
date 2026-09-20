package rag_http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/infra/httpclient"
)

type SearchIndexerClient struct {
	BaseURL string
	Client  *http.Client
}

// NewSearchIndexerClient constructs a client for search-indexer's REST API.
// Authentication is established at the TLS transport layer (mTLS). The third
// argument is retained for DI-signature compatibility and ignored.
func NewSearchIndexerClient(baseURL string, timeout int, _ string) *SearchIndexerClient {
	return &SearchIndexerClient{
		BaseURL: baseURL,
		Client:  httpclient.NewPooledClient(time.Duration(timeout) * time.Second),
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

// Search runs a user-scoped article search for tool/tag-extraction paths.
//
// user_id is required. search-indexer scopes hits to that user's indexed documents
// (Meilisearch user_id = "..."). Empty user_id fails fast to avoid unscoped search.
func (c *SearchIndexerClient) Search(ctx context.Context, query string, userID string) ([]domain.SearchHit, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("search_indexer_client: user_id is required")
	}

	u, err := url.Parse(fmt.Sprintf("%s/v1/search", c.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	q := u.Query()
	q.Set("q", query)
	q.Set("user_id", strings.TrimSpace(userID))
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

// SearchBM25 performs user-scoped BM25 (keyword) search for hybrid search fusion.
// Implements domain.BM25Searcher interface.
//
// user_id is required; an empty user_id fails fast rather than searching unscoped.
func (c *SearchIndexerClient) SearchBM25(ctx context.Context, query string, limit int, userID string) ([]domain.BM25SearchResult, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("search_indexer_client: user_id is required")
	}

	u, err := url.Parse(fmt.Sprintf("%s/v1/search", c.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	q := u.Query()
	q.Set("q", query)
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("user_id", strings.TrimSpace(userID))
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
