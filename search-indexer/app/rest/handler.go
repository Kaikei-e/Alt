package rest

import (
	"encoding/json"
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
		resp.Hits = append(resp.Hits, SearchArticlesHit{
			ID:      h.(map[string]interface{})["id"].(string),
			Title:   h.(map[string]interface{})["title"].(string),
			Content: h.(map[string]interface{})["content"].(string),
			Tags:    toStringSlice(h.(map[string]interface{})["tags"]),
		})
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
