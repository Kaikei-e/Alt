package rest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"search-indexer/domain"
	"search-indexer/logger"
	"search-indexer/port"
	"search-indexer/usecase"
	appOtel "search-indexer/utils/otel"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Handler contains all HTTP handlers for the search indexer.
// ``engine`` is retained so the date-bounded search path can call
// SearchWithDateFilter directly without introducing another usecase.
type Handler struct {
	searchByUserUsecase   *usecase.SearchByUserUsecase
	searchArticlesUsecase *usecase.SearchArticlesUsecase
	engine                port.SearchEngine
}

// NewHandler creates a new Handler. ``engine`` is derived from the same
// port.SearchEngine that backs the two usecases.
func NewHandler(searchByUserUsecase *usecase.SearchByUserUsecase, searchArticlesUsecase *usecase.SearchArticlesUsecase) *Handler {
	return &Handler{
		searchByUserUsecase:   searchByUserUsecase,
		searchArticlesUsecase: searchArticlesUsecase,
		engine:                searchArticlesUsecase.Engine(),
	}
}

type SearchArticlesHit struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags"`
	Score       float64  `json:"score"`
	Language    string   `json:"language,omitempty"`
	PublishedAt string   `json:"published_at,omitempty"`
}

type SearchArticlesResponse struct {
	Query string              `json:"query"`
	Hits  []SearchArticlesHit `json:"hits"`
	Total int                 `json:"total"`
}

// SearchArticles handles GET /v1/search requests.
// When user_id is provided, results are filtered to that user's articles.
// When user_id is omitted, all articles are searched (used by RAG/BM25 internal callers).
// Optional ``published_after`` / ``published_before`` RFC3339 parameters
// restrict results to a date window. Both bounds apply to the ``published_at``
// attribute on indexed documents.
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

	publishedAfter, err := parseOptionalRFC3339(r.URL.Query().Get("published_after"))
	if err != nil {
		http.Error(w, "invalid published_after (expected RFC3339)", http.StatusBadRequest)
		return
	}
	publishedBefore, err := parseOptionalRFC3339(r.URL.Query().Get("published_before"))
	if err != nil {
		http.Error(w, "invalid published_before (expected RFC3339)", http.StatusBadRequest)
		return
	}

	var docs []domain.SearchDocument
	var searchQuery string

	if userID == "" {
		// Unfiltered search for internal RAG/BM25 callers
		limit := 50
		if limitStr != "" {
			if l, parseErr := strconv.Atoi(limitStr); parseErr == nil && l > 0 && l <= 1000 {
				limit = l
			}
		}
		if publishedAfter != nil || publishedBefore != nil {
			dateDocs, dfErr := h.engine.SearchWithDateFilter(ctx, query, publishedAfter, publishedBefore, limit)
			if dfErr != nil {
				err = dfErr
			} else {
				docs = dateDocs
				searchQuery = query
			}
		} else {
			result, execErr := h.searchArticlesUsecase.Execute(ctx, query, limit)
			if execErr != nil {
				err = execErr
			} else {
				docs = result.Documents
				searchQuery = result.Query
			}
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
		logger.Logger.ErrorContext(ctx, "search failed", "err", err, "user_id", userID, "query_hash", logger.HashQuery(query))
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
		Total: len(docs),
	}

	for _, doc := range docs {
		tags := doc.Tags
		if tags == nil {
			tags = []string{}
		}
		publishedAt := ""
		if !doc.PublishedAt.IsZero() {
			publishedAt = doc.PublishedAt.UTC().Format(time.RFC3339)
		}
		resp.Hits = append(resp.Hits, SearchArticlesHit{
			ID:          doc.ID,
			Title:       doc.Title,
			Content:     doc.Content,
			Tags:        tags,
			Score:       doc.Score,
			Language:    doc.Language,
			PublishedAt: publishedAt,
		})
	}

	logger.Logger.InfoContext(ctx, "search ok", "query_hash", logger.HashQuery(query), "user_id", userID, "count", len(resp.Hits))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Logger.ErrorContext(ctx, "encode failed", "err", err)
	}
}

// parseOptionalRFC3339 returns a *time.Time when the raw value is non-empty,
// a parse error when it is invalid, and (nil, nil) when the caller omitted it.
func parseOptionalRFC3339(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

