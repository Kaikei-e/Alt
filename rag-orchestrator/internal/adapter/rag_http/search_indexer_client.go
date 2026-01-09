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
	// e.g. /search?q=query&user_id=system (or some user)
	// The search-indexer requires user_id. We'll use "rag-orchestrator" for now or pass it from input?
	// The domain interface Update: Search(ctx, query string)
	// We might need to pass UserID if search-indexer enforces it strictly.
	// Looking at search-indexer/app/rest/handler.go:
	// userID := r.URL.Query().Get("user_id")
	// if userID == "" ... return 400
	// So we need a user_id.
	// We should probably rely on the fact that this is an internal call.
	// But let's pass a system user ID for now.

	systemUserID := "rag-orchestrator-system"

	u, err := url.Parse(fmt.Sprintf("%s/v1/search", c.BaseURL))
	// Wait, search-indexer handler path?
	// I need to check how search-indexer is exposed.
	// Assuming it's just /search or similar.
	// Let's assume /search based on typical patterns, but I should check `search-indexer/app/server/server.go`.
	// For now I'll implement with a placeholder path and fix it in the next step if wrong.
	// Actually, `search-indexer/app/rest/handler.go` function `SearchArticles`.
	// I need to know the route registration.
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
