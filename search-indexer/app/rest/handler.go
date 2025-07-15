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

// safeExtractSearchHit safely extracts SearchArticlesHit from interface{}
func safeExtractSearchHit(hit interface{}) (SearchArticlesHit, error) {
	hitMap, ok := hit.(map[string]interface{})
	if !ok {
		return SearchArticlesHit{}, fmt.Errorf("invalid hit format: expected map[string]interface{}, got %T", hit)
	}

	// Extract and validate ID
	idRaw, exists := hitMap["id"]
	if !exists {
		return SearchArticlesHit{}, fmt.Errorf("missing required field: id")
	}
	id, ok := idRaw.(string)
	if !ok {
		return SearchArticlesHit{}, fmt.Errorf("invalid id type: expected string, got %T", idRaw)
	}

	// Extract and validate Title
	titleRaw, exists := hitMap["title"]
	if !exists {
		return SearchArticlesHit{}, fmt.Errorf("missing required field: title")
	}
	title, ok := titleRaw.(string)
	if !ok {
		return SearchArticlesHit{}, fmt.Errorf("invalid title type: expected string, got %T", titleRaw)
	}

	// Extract and validate Content
	contentRaw, exists := hitMap["content"]
	if !exists {
		return SearchArticlesHit{}, fmt.Errorf("missing required field: content")
	}
	content, ok := contentRaw.(string)
	if !ok {
		return SearchArticlesHit{}, fmt.Errorf("invalid content type: expected string, got %T", contentRaw)
	}

	// Extract tags (optional field, use existing toStringSlice function)
	tagsRaw, exists := hitMap["tags"]
	var tags []string
	if exists {
		tags = toStringSlice(tagsRaw)
	}

	return SearchArticlesHit{
		ID:      id,
		Title:   title,
		Content: content,
		Tags:    tags,
	}, nil
}

func SearchArticles(
	w http.ResponseWriter,
	r *http.Request,
	idx meilisearch.IndexManager,
) {
	query := r.URL.Query().Get("q") // クエリキーを 'q' に統一

	if query == "" {
		logger.Logger.Error("query is empty")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	raw, err := search_engine.SearchArticles(idx, query)
	if err != nil {
		logger.Logger.Error("search failed", "err", err)
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
			logger.Logger.Error("failed to extract search hit", "err", err)
			continue // Skip invalid hits instead of failing the entire request
		}
		resp.Hits = append(resp.Hits, hit)
	}

	logger.Logger.Info("search ok", "query", query, "count", len(resp.Hits))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Logger.Error("encode failed", "err", err)
	}
}

func toStringSlice(v interface{}) []string {
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
