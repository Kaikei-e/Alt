package global_search_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/global_search_port"
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TracerName identifies the tracer the composition root must create and
// inject via NewGlobalSearchUsecase (usecase layer depends on the trace.Tracer
// port only, never on the global otel registry).
const TracerName = "alt-backend/global_search"

const (
	defaultArticleLimit = 5
	defaultRecapLimit   = 3
	defaultTagLimit     = 10
	maxArticleLimit     = 20
	maxRecapLimit       = 10
	maxTagLimit         = 30
	maxQueryLength      = 1000
)

// sectionTimeouts caps each parallel search section independently. The
// previous single 10s ceiling was a worst-case shock absorber for hybrid-
// search cold-start (~1.1s embed) + saturated synonym task queue. Now that
// the search-indexer warms the embedder at startup, the LRU cache absorbs
// repeat traffic, and Meilisearch enforces searchCutoffMs=1500, the article
// path consistently stays under 2s; recaps are similar. Tags resolve via a
// short alt-db prefix query and never need more than a second.
//
// Lowering the ceilings tightens user-visible latency caps on the slow path
// without compromising the cache-hit fast path.
//
// Kept as a package var (not a const) so tests can shrink the timeouts to
// reproduce degraded scenarios in milliseconds.
var sectionTimeouts = struct {
	Articles, Recaps, Tags time.Duration
}{
	Articles: 5 * time.Second,
	Recaps:   5 * time.Second,
	Tags:     1 * time.Second,
}

// GlobalSearchUsecase aggregates search results from multiple verticals.
type GlobalSearchUsecase struct {
	articleSearch global_search_port.SearchArticlesPort
	recapSearch   global_search_port.SearchRecapsPort
	tagSearch     global_search_port.SearchTagsPort
	logger        *slog.Logger
	tracer        trace.Tracer
}

// NewGlobalSearchUsecase creates a new GlobalSearchUsecase.
func NewGlobalSearchUsecase(
	articleSearch global_search_port.SearchArticlesPort,
	recapSearch global_search_port.SearchRecapsPort,
	tagSearch global_search_port.SearchTagsPort,
	tracer trace.Tracer,
) *GlobalSearchUsecase {
	return &GlobalSearchUsecase{
		articleSearch: articleSearch,
		recapSearch:   recapSearch,
		tagSearch:     tagSearch,
		logger:        slog.Default(),
		tracer:        tracer,
	}
}

// Execute performs a federated search across all content verticals.
func (u *GlobalSearchUsecase) Execute(ctx context.Context, query string, articleLimit, recapLimit, tagLimit int) (*domain.GlobalSearchResult, error) {
	if query == "" {
		return nil, errors.New("query is required")
	}
	if len(query) > maxQueryLength {
		return nil, errors.New("query exceeds maximum length")
	}

	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, errors.New("user context not found")
	}

	articleLimit = clampLimit(articleLimit, defaultArticleLimit, maxArticleLimit)
	recapLimit = clampLimit(recapLimit, defaultRecapLimit, maxRecapLimit)
	tagLimit = clampLimit(tagLimit, defaultTagLimit, maxTagLimit)

	var (
		mu               sync.Mutex
		articles         *domain.ArticleSearchSection
		recaps           *domain.RecapSearchSection
		tags             *domain.TagSearchSection
		degradedSections []string
		wg               sync.WaitGroup
	)

	userID := user.UserID.String()

	// Article search
	wg.Add(1)
	go func() {
		defer wg.Done()
		sctx, cancel := context.WithTimeout(ctx, sectionTimeouts.Articles)
		defer cancel()
		sctx, span := u.tracer.Start(sctx, "global_search.section.articles",
			trace.WithAttributes(attribute.Int("limit", articleLimit)))
		start := time.Now()
		degraded := false
		defer func() {
			span.SetAttributes(
				attribute.Int64("duration_ms", time.Since(start).Milliseconds()),
				attribute.Bool("degraded", degraded),
			)
			span.End()
		}()

		result, err := u.articleSearch.SearchArticlesForGlobal(sctx, query, userID, articleLimit)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			degraded = true
			u.logger.WarnContext(ctx, "article search failed", "error", err, "query", query)
			degradedSections = append(degradedSections, "articles")
			return
		}
		articles = result
	}()

	// Recap search
	wg.Add(1)
	go func() {
		defer wg.Done()
		sctx, cancel := context.WithTimeout(ctx, sectionTimeouts.Recaps)
		defer cancel()
		sctx, span := u.tracer.Start(sctx, "global_search.section.recaps",
			trace.WithAttributes(attribute.Int("limit", recapLimit)))
		start := time.Now()
		degraded := false
		defer func() {
			span.SetAttributes(
				attribute.Int64("duration_ms", time.Since(start).Milliseconds()),
				attribute.Bool("degraded", degraded),
			)
			span.End()
		}()

		result, err := u.recapSearch.SearchRecapsForGlobal(sctx, query, recapLimit)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			degraded = true
			u.logger.WarnContext(ctx, "recap search failed", "error", err, "query", query)
			degradedSections = append(degradedSections, "recaps")
			return
		}
		recaps = result
	}()

	// Tag search
	wg.Add(1)
	go func() {
		defer wg.Done()
		sctx, cancel := context.WithTimeout(ctx, sectionTimeouts.Tags)
		defer cancel()
		sctx, span := u.tracer.Start(sctx, "global_search.section.tags",
			trace.WithAttributes(attribute.Int("limit", tagLimit)))
		start := time.Now()
		degraded := false
		defer func() {
			span.SetAttributes(
				attribute.Int64("duration_ms", time.Since(start).Milliseconds()),
				attribute.Bool("degraded", degraded),
			)
			span.End()
		}()

		result, err := u.tagSearch.SearchTagsByPrefix(sctx, query, tagLimit)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			degraded = true
			u.logger.WarnContext(ctx, "tag search failed", "error", err, "query", query)
			degradedSections = append(degradedSections, "tags")
			return
		}
		tags = result
	}()

	wg.Wait()

	// If all sections failed, return an error
	if len(degradedSections) == 3 {
		return nil, errors.New("all search sections failed")
	}

	return &domain.GlobalSearchResult{
		Query:            query,
		Articles:         articles,
		Recaps:           recaps,
		Tags:             tags,
		DegradedSections: degradedSections,
		SearchedAt:       time.Now(),
	}, nil
}

func clampLimit(value, defaultVal, maxVal int) int {
	if value <= 0 {
		return defaultVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}
