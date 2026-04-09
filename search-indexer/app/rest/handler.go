package rest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"search-indexer/domain"
	"search-indexer/logger"
	"search-indexer/usecase"
	appOtel "search-indexer/utils/otel"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Handler contains all HTTP handlers for the search indexer
type Handler struct {
	searchByUserUsecase   *usecase.SearchByUserUsecase
	searchArticlesUsecase *usecase.SearchArticlesUsecase
}

// NewHandler creates a new Handler
func NewHandler(searchByUserUsecase *usecase.SearchByUserUsecase, searchArticlesUsecase *usecase.SearchArticlesUsecase) *Handler {
	return &Handler{
		searchByUserUsecase:   searchByUserUsecase,
		searchArticlesUsecase: searchArticlesUsecase,
	}
}

type SearchArticlesHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	Score   float64  `json:"score"`
}

type SearchArticlesResponse struct {
	Query string              `json:"query"`
	Hits  []SearchArticlesHit `json:"hits"`
}

// SearchArticles handles GET /v1/search requests.
// When user_id is provided, results are filtered to that user's articles.
// When user_id is omitted, all articles are searched (used by RAG/BM25 internal callers).
func (h *Handler) SearchArticles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()
	query := r.URL.Query().Get("q")
	userID := r.URL.Query().Get("user_id")
	limitStr := r.URL.Query().Get("limit")

	if query == "" {
		logger.Logger.ErrorContext(ctx, "query is empty")
		http.Error(w, "query parameter required", http.StatusBadRequest)
		return
	}

	var docs []domain.SearchDocument
	var searchQuery string
	var err error

	if userID == "" {
		// Unfiltered search for internal RAG/BM25 callers
		limit := 50
		if limitStr != "" {
			if l, parseErr := strconv.Atoi(limitStr); parseErr == nil && l > 0 && l <= 1000 {
				limit = l
			}
		}
		result, execErr := h.searchArticlesUsecase.Execute(ctx, query, limit)
		if execErr != nil {
			err = execErr
		} else {
			docs = result.Documents
			searchQuery = result.Query
		}
	} else {
		// User-scoped search
		result, execErr := h.searchByUserUsecase.Execute(ctx, query, userID)
		if execErr != nil {
			err = execErr
		} else {
			docs = result.Hits
			searchQuery = result.Query
		}
	}

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
		Query: searchQuery,
		Hits:  make([]SearchArticlesHit, 0, len(docs)),
	}

	for _, doc := range docs {
		tags := doc.Tags
		if tags == nil {
			tags = []string{}
		}
		resp.Hits = append(resp.Hits, SearchArticlesHit{
			ID:      doc.ID,
			Title:   doc.Title,
			Content: doc.Content,
			Tags:    tags,
			Score:   doc.Score,
		})
	}

	logger.Logger.InfoContext(ctx, "search ok", "query", query, "user_id", userID, "count", len(resp.Hits))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Logger.ErrorContext(ctx, "encode failed", "err", err)
	}
}
