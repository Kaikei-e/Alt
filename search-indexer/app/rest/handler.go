package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"search-indexer/logger"
	"search-indexer/usecase"
	appOtel "search-indexer/utils/otel"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Handler contains all HTTP handlers for the search indexer
type Handler struct {
	searchByUserUsecase *usecase.SearchByUserUsecase
}

// NewHandler creates a new Handler
func NewHandler(searchByUserUsecase *usecase.SearchByUserUsecase) *Handler {
	return &Handler{
		searchByUserUsecase: searchByUserUsecase,
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

// SearchArticles handles GET /v1/search requests.
func (h *Handler) SearchArticles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()
	query := r.URL.Query().Get("q")
	userID := r.URL.Query().Get("user_id")

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

	result, err := h.searchByUserUsecase.Execute(ctx, query, userID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "search failed", "err", err, "user_id", userID)
		if m := appOtel.Metrics; m != nil {
			m.ErrorsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "search")))
		}
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if m := appOtel.Metrics; m != nil {
		m.SearchDuration.Record(ctx, time.Since(start).Seconds())
	}

	resp := SearchArticlesResponse{
		Query: result.Query,
		Hits:  make([]SearchArticlesHit, 0, len(result.Hits)),
	}

	for _, doc := range result.Hits {
		tags := doc.Tags
		if tags == nil {
			tags = []string{}
		}
		resp.Hits = append(resp.Hits, SearchArticlesHit{
			ID:      doc.ID,
			Title:   doc.Title,
			Content: doc.Content,
			Tags:    tags,
		})
	}

	logger.Logger.InfoContext(ctx, "search ok", "query", query, "user_id", userID, "count", len(resp.Hits))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Logger.ErrorContext(ctx, "encode failed", "err", err)
	}
}
