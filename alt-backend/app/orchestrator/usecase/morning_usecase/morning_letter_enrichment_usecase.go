package morning_usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"alt/domain"
	"alt/orchestrator/port/morning_letter_port"

	"github.com/google/uuid"
)

// enrichmentMaxSources caps the fan-out (article fetch + related search)
// per Letter so a 257-article overnight doesn't trigger a 257-call storm.
// Letters rarely need more than the top-N cards for browsing.
const enrichmentMaxSources = 8

// enrichmentRelatedPerBullet is how many related-article teasers we
// attempt to attach per source. search-indexer returns more; we keep
// only this many so the UI stays compact.
const enrichmentRelatedPerBullet = 3

// enrichmentSearchConcurrency bounds parallel search-indexer calls.
const enrichmentSearchConcurrency = 4

// summaryExcerptChars caps the inline preview length.
const summaryExcerptChars = 220

// Execute-style enrichment: pulls the sources for a Letter, batch-fetches
// articles + feed titles, concurrently fetches related articles from
// search-indexer, and returns one domain.MorningLetterBulletEnrichment
// per source (capped at enrichmentMaxSources).
func (u *morningLetterUsecase) GetLetterEnrichment(
	ctx context.Context,
	letterID, userID string,
) ([]*domain.MorningLetterBulletEnrichment, error) {
	if strings.TrimSpace(letterID) == "" {
		return nil, fmt.Errorf("letter_id required for enrichment")
	}
	if u.articleBatch == nil {
		return nil, fmt.Errorf("enrichment disabled: article batch port unset")
	}

	// 1. Sources (already subscription-filtered inside GetLetterSources).
	sources, err := u.GetLetterSources(ctx, letterID)
	if err != nil {
		return nil, fmt.Errorf("load sources: %w", err)
	}
	if len(sources) == 0 {
		return []*domain.MorningLetterBulletEnrichment{}, nil
	}
	if len(sources) > enrichmentMaxSources {
		sources = sources[:enrichmentMaxSources]
	}

	// 2. Batch-fetch articles (URL, title, tags, feed_id via existing driver).
	articleIDs := make([]uuid.UUID, 0, len(sources))
	seenArticle := make(map[uuid.UUID]struct{}, len(sources))
	for _, s := range sources {
		if _, dup := seenArticle[s.ArticleID]; dup {
			continue
		}
		seenArticle[s.ArticleID] = struct{}{}
		articleIDs = append(articleIDs, s.ArticleID)
	}
	articles, err := u.articleBatch.FetchArticlesByIDs(ctx, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch articles: %w", err)
	}
	articleByID := make(map[uuid.UUID]*domain.Article, len(articles))
	feedIDs := make([]uuid.UUID, 0, len(articles))
	feedSeen := make(map[uuid.UUID]struct{}, len(articles))
	for _, a := range articles {
		articleByID[a.ID] = a
		if _, dup := feedSeen[a.FeedID]; !dup && a.FeedID != uuid.Nil {
			feedSeen[a.FeedID] = struct{}{}
			feedIDs = append(feedIDs, a.FeedID)
		}
	}

	// 3. Batch-resolve feed titles (optional — missing is fine).
	feedTitleByID := map[uuid.UUID]string{}
	if u.feedTitleBatch != nil && len(feedIDs) > 0 {
		if m, ferr := u.feedTitleBatch.FetchFeedTitlesByIDs(ctx, feedIDs); ferr == nil {
			feedTitleByID = m
		}
	}

	// 4. Fan-out: related articles from search-indexer, concurrency-capped.
	relatedByID := fetchRelatedArticles(
		ctx,
		u.searchRelated,
		articles,
		userID,
		enrichmentSearchConcurrency,
		enrichmentRelatedPerBullet,
	)

	// 4b. Hydrate related-article URL/title from Alt DB so the teaser
	// links carry the ?url&title query params required by /articles/[id].
	// search-indexer doesn't return URLs; the DB is the source of truth.
	relatedMetaByID := hydrateRelatedArticleMeta(ctx, u.articleBatch, relatedByID)

	// 5. Assemble per-source enrichments, in the same order as `sources`.
	return assembleBulletEnrichments(sources, articleByID, feedTitleByID, relatedByID, relatedMetaByID), nil
}

// hydrateRelatedArticleMeta batch-fetches articles for every unique
// related-article id discovered across all sources, returning a lookup
// keyed by uuid string. Missing rows are silently skipped — the caller
// falls back to bare /articles/<id> for those teasers.
func hydrateRelatedArticleMeta(
	ctx context.Context,
	articleBatch morning_letter_port.ArticleMetadataBatchPort,
	relatedByID map[uuid.UUID][]domain.RelatedArticleTeaser,
) map[string]*domain.Article {
	if articleBatch == nil || len(relatedByID) == 0 {
		return map[string]*domain.Article{}
	}
	seen := map[uuid.UUID]struct{}{}
	ids := make([]uuid.UUID, 0)
	for _, teasers := range relatedByID {
		for _, t := range teasers {
			id, err := uuid.Parse(t.ArticleID)
			if err != nil {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return map[string]*domain.Article{}
	}
	articles, err := articleBatch.FetchArticlesByIDs(ctx, ids)
	if err != nil {
		return map[string]*domain.Article{}
	}
	out := make(map[string]*domain.Article, len(articles))
	for _, a := range articles {
		out[a.ID.String()] = a
	}
	return out
}

// fetchRelatedArticles runs bounded-concurrent search-indexer calls keyed
// by article title, then converts hits to teasers. Returns a map keyed
// by the seed article's id.
func fetchRelatedArticles(
	ctx context.Context,
	searchPort morning_letter_port.SearchRelatedArticlesPort,
	articles []*domain.Article,
	userID string,
	concurrency int,
	perBullet int,
) map[uuid.UUID][]domain.RelatedArticleTeaser {
	result := make(map[uuid.UUID][]domain.RelatedArticleTeaser, len(articles))
	if searchPort == nil || len(articles) == 0 {
		return result
	}

	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, art := range articles {
		if strings.TrimSpace(art.Title) == "" {
			continue
		}
		a := art
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			hits, err := searchPort.SearchArticles(ctx, a.Title, userID)
			if err != nil || len(hits) == 0 {
				return
			}
			teasers := make([]domain.RelatedArticleTeaser, 0, perBullet)
			for _, h := range hits {
				if h.ID == a.ID.String() {
					continue // don't recommend the seed article itself
				}
				teasers = append(teasers, domain.RelatedArticleTeaser{
					ArticleID:      h.ID,
					Title:          h.Title,
					ArticleAltHref: fmt.Sprintf("/articles/%s", h.ID),
				})
				if len(teasers) >= perBullet {
					break
				}
			}
			mu.Lock()
			result[a.ID] = teasers
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}
