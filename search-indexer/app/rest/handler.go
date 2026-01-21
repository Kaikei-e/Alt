package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"search-indexer/logger"
	"search-indexer/search_engine"
	"search-indexer/usecase"

	"github.com/meilisearch/meilisearch-go"
)

// Handler contains all HTTP handlers for the search indexer
type Handler struct {
	searchUsecase *usecase.SearchArticlesUsecase
}

// NewHandler creates a new Handler
func NewHandler(searchUsecase *usecase.SearchArticlesUsecase) *Handler {
	return &Handler{
		searchUsecase: searchUsecase,
	}
}

type SearchArticlesHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type SearchArticlesResponse struct {
	Query string              `json:"query"`
	Hits  []SearchArticlesHit `json:"hits"`
}

// safeExtractSearchHit safely extracts SearchArticlesHit from meilisearch.Hit
func safeExtractSearchHit(hit meilisearch.Hit) (SearchArticlesHit, error) {
	// meilisearch.Hit is map[string]json.RawMessage, need to unmarshal each field
	var result SearchArticlesHit

	// Extract ID
	if idBytes, exists := hit["id"]; exists {
		if err := json.Unmarshal(idBytes, &result.ID); err != nil {
			return SearchArticlesHit{}, fmt.Errorf("failed to unmarshal id: %w", err)
		}
	} else {
		return SearchArticlesHit{}, fmt.Errorf("missing required field: id")
	}

	// Extract Title
	if titleBytes, exists := hit["title"]; exists {
		if err := json.Unmarshal(titleBytes, &result.Title); err != nil {
			return SearchArticlesHit{}, fmt.Errorf("failed to unmarshal title: %w", err)
		}
	} else {
		return SearchArticlesHit{}, fmt.Errorf("missing required field: title")
	}

	// Extract Content
	if contentBytes, exists := hit["content"]; exists {
		if err := json.Unmarshal(contentBytes, &result.Content); err != nil {
			return SearchArticlesHit{}, fmt.Errorf("failed to unmarshal content: %w", err)
		}
	} else {
		return SearchArticlesHit{}, fmt.Errorf("missing required field: content")
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

	return result, nil
}

func SearchArticles(
	w http.ResponseWriter,
	r *http.Request,
	idx meilisearch.IndexManager,
) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")        // クエリキーを 'q' に統一
	userID := r.URL.Query().Get("user_id") // user_idパラメータ

	if query == "" {
		logger.Logger.ErrorContext(ctx, "query is empty")
		http.Error(w, "query parameter required", http.StatusBadRequest)
		return
	}

	if userID == "" {
		logger.Logger.ErrorContext(ctx, "user_id is empty")
		http.Error(w, "user_id parameter required", http.StatusBadRequest)
		return
	}

	// Build filter for user_id using secure escaping
	filter := fmt.Sprintf("user_id = \"%s\"", search_engine.EscapeMeilisearchValue(userID))

	raw, err := search_engine.SearchArticlesWithFilter(idx, query, filter)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "search failed", "err", err, "user_id", userID)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	resp := SearchArticlesResponse{
		Query: query,
		Hits:  make([]SearchArticlesHit, 0, len(raw.Hits)),
	}

	for _, h := range raw.Hits {
		hit, err := safeExtractSearchHit(h)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to extract search hit", "err", err)
			continue // Skip invalid hits instead of failing the entire request
		}
		resp.Hits = append(resp.Hits, hit)
	}

	logger.Logger.InfoContext(ctx, "search ok", "query", query, "user_id", userID, "count", len(resp.Hits))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Logger.ErrorContext(ctx, "encode failed", "err", err)
	}
}
