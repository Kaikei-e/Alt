package global_search_usecase

import (
	"alt/domain"
	"alt/port/global_search_port"
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultArticleLimit = 5
	defaultRecapLimit   = 3
	defaultTagLimit     = 10
	maxArticleLimit     = 20
	maxRecapLimit       = 10
	maxTagLimit         = 30
	maxQueryLength      = 1000
)

// sectionTimeout bounds each parallel search section (articles / recaps /
// tags). It is a var rather than a const so tests can shrink it for fast
// behavioural assertions.
var sectionTimeout = 3 * time.Second

// GlobalSearchUsecase aggregates search results from multiple verticals.
type GlobalSearchUsecase struct {
	articleSearch global_search_port.SearchArticlesPort
	recapSearch   global_search_port.SearchRecapsPort
	tagSearch     global_search_port.SearchTagsPort
	logger        *slog.Logger
}

// NewGlobalSearchUsecase creates a new GlobalSearchUsecase.
func NewGlobalSearchUsecase(
	articleSearch global_search_port.SearchArticlesPort,
	recapSearch global_search_port.SearchRecapsPort,
	tagSearch global_search_port.SearchTagsPort,
) *GlobalSearchUsecase {
	return &GlobalSearchUsecase{
		articleSearch: articleSearch,
		recapSearch:   recapSearch,
		tagSearch:     tagSearch,
		logger:        slog.Default(),
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
		sctx, cancel := context.WithTimeout(ctx, sectionTimeout)
		defer cancel()

		result, err := u.articleSearch.SearchArticlesForGlobal(sctx, query, userID, articleLimit)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
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
		sctx, cancel := context.WithTimeout(ctx, sectionTimeout)
		defer cancel()

		result, err := u.recapSearch.SearchRecapsForGlobal(sctx, query, recapLimit)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
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
		sctx, cancel := context.WithTimeout(ctx, sectionTimeout)
		defer cancel()

		result, err := u.tagSearch.SearchTagsByPrefix(sctx, query, tagLimit)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
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
