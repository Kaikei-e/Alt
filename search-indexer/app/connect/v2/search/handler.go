// Package search provides Connect-RPC handlers for search operations.
package search

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	"github.com/meilisearch/meilisearch-go"

	searchv2 "search-indexer/gen/proto/services/search/v2"
	"search-indexer/gen/proto/services/search/v2/searchv2connect"
	"search-indexer/logger"
	"search-indexer/search_engine"
)

// Handler implements the SearchService Connect-RPC handler.
type Handler struct {
	idx meilisearch.IndexManager
}

// NewHandler creates a new search handler.
func NewHandler(idx meilisearch.IndexManager) *Handler {
	return &Handler{idx: idx}
}

// Compile-time check that Handler implements SearchServiceHandler.
var _ searchv2connect.SearchServiceHandler = (*Handler)(nil)

// SearchArticles searches for articles matching the query.
func (h *Handler) SearchArticles(
	ctx context.Context,
	req *connect.Request[searchv2.SearchArticlesRequest],
) (*connect.Response[searchv2.SearchArticlesResponse], error) {
	query := req.Msg.Query
	userID := req.Msg.UserId

	// Validate required parameters
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}

	// Build filter for user_id using secure escaping
	filter := fmt.Sprintf("user_id = \"%s\"", search_engine.EscapeMeilisearchValue(userID))

	raw, err := search_engine.SearchArticlesWithFilter(h.idx, query, filter)
	if err != nil {
		logger.Logger.Error("search failed", "err", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("search failed"))
	}

	hits := make([]*searchv2.SearchHit, 0, len(raw.Hits))
	for _, hit := range raw.Hits {
		searchHit, err := extractSearchHit(hit)
		if err != nil {
			logger.Logger.Error("failed to extract search hit", "err", err)
			continue // Skip invalid hits
		}
		hits = append(hits, searchHit)
	}

	logger.Logger.Info("search ok", "query", query, "user_id", userID, "count", len(hits))

	return connect.NewResponse(&searchv2.SearchArticlesResponse{
		Query: query,
		Hits:  hits,
	}), nil
}

// extractSearchHit safely extracts SearchHit from meilisearch.Hit
func extractSearchHit(hit meilisearch.Hit) (*searchv2.SearchHit, error) {
	var result searchv2.SearchHit

	// Extract ID
	if idBytes, exists := hit["id"]; exists {
		if err := json.Unmarshal(idBytes, &result.Id); err != nil {
			return nil, fmt.Errorf("failed to unmarshal id: %w", err)
		}
	} else {
		return nil, fmt.Errorf("missing required field: id")
	}

	// Extract Title
	if titleBytes, exists := hit["title"]; exists {
		if err := json.Unmarshal(titleBytes, &result.Title); err != nil {
			return nil, fmt.Errorf("failed to unmarshal title: %w", err)
		}
	} else {
		return nil, fmt.Errorf("missing required field: title")
	}

	// Extract Content
	if contentBytes, exists := hit["content"]; exists {
		if err := json.Unmarshal(contentBytes, &result.Content); err != nil {
			return nil, fmt.Errorf("failed to unmarshal content: %w", err)
		}
	} else {
		return nil, fmt.Errorf("missing required field: content")
	}

	// Extract Tags (optional)
	if tagsBytes, exists := hit["tags"]; exists {
		if err := json.Unmarshal(tagsBytes, &result.Tags); err != nil {
			// Tags is optional, so just set empty array on error
			result.Tags = []string{}
		}
	} else {
		result.Tags = []string{}
	}

	return &result, nil
}
