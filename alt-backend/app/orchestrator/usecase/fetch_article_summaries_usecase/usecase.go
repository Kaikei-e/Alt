// Package fetch_article_summaries_usecase orchestrates the batch
// "resolve-or-fetch up to 50 articles, then get each one's AI summary" flow
// used by POST /v1/feeds/fetch/summary. It replaces handler code that
// called container.AltDBRepository (a driver) directly for both existence
// checks and summary persistence.
package fetch_article_summaries_usecase

import (
	"alt/domain"
	"alt/utils/batch_article_fetcher"
	"alt/utils/logger"
	"context"
	"regexp"
	"strings"
)

// ArticleRepository is the narrow slice of the article table this usecase
// needs. It is satisfied structurally by *alt_db.AltDBRepository.
type ArticleRepository interface {
	FetchArticlesByURLs(ctx context.Context, urls []string) (map[string]*domain.ArticleContent, error)
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
}

// ArticleBatchFetcher fetches multiple article URLs concurrently with
// per-domain rate limiting. Satisfied structurally by
// *batch_article_fetcher.BatchArticleFetcher.
type ArticleBatchFetcher interface {
	FetchMultiple(ctx context.Context, urls []string) map[string]*batch_article_fetcher.FetchResult
}

// ArticleSummarizer resolves the AI summary for a single already-persisted
// article, generating and saving one if it is not already cached.
// Satisfied structurally by *summarize_article_usecase.Usecase.
type ArticleSummarizer interface {
	EnsureSummary(ctx context.Context, articleID, userID, title string) (summary string, fromCache bool, err error)
}

// Result is a single resolved article summary.
type Result struct {
	FeedURL   string
	ArticleID string
	Title     string
	Summary   string
}

// Usecase resolves up to 50 feed URLs to persisted articles (fetching from
// the web when needed) and returns each one's AI summary.
type Usecase struct {
	repo       ArticleRepository
	fetcher    ArticleBatchFetcher
	summarizer ArticleSummarizer
}

// NewUsecase creates a Usecase.
func NewUsecase(repo ArticleRepository, fetcher ArticleBatchFetcher, summarizer ArticleSummarizer) *Usecase {
	return &Usecase{repo: repo, fetcher: fetcher, summarizer: summarizer}
}

type articleInfo struct {
	id     string
	title  string
	exists bool
	err    error
}

// Execute resolves feedURLs and returns the summary for each one that could
// be resolved. URLs that fail to resolve (fetch error, save error, missing
// article) are logged and skipped, matching the legacy handler's
// continue-on-error behavior.
func (u *Usecase) Execute(ctx context.Context, userID string, feedURLs []string) []Result {
	articles := u.resolveArticles(ctx, feedURLs)

	results := make([]Result, 0, len(feedURLs))
	for _, feedURL := range feedURLs {
		info, ok := articles[feedURL]
		if !ok || info.err != nil || !info.exists || info.id == "" {
			if ok && info.err != nil {
				logger.Logger.ErrorContext(ctx, "skipping url due to resolution error", "url", feedURL, "error", info.err)
			}
			continue
		}

		summary, fromCache, err := u.summarizer.EnsureSummary(ctx, info.id, userID, info.title)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to summarize article", "error", err, "url", feedURL, "article_id", info.id)
			continue
		}

		results = append(results, Result{
			FeedURL:   feedURL,
			ArticleID: info.id,
			Title:     info.title,
			Summary:   cleanSummaryContent(summary),
		})

		logger.Logger.InfoContext(ctx, "article summary processed", "feed_url", feedURL, "from_cache", fromCache)
	}

	return results
}

// resolveArticles checks the database for feedURLs in a single batch query
// (avoiding an N+1 round-trip per URL) and batch-fetches (then saves) the
// ones that are missing.
func (u *Usecase) resolveArticles(ctx context.Context, feedURLs []string) map[string]*articleInfo {
	articles := make(map[string]*articleInfo, len(feedURLs))

	existing, err := u.repo.FetchArticlesByURLs(ctx, feedURLs)
	if err != nil {
		for _, feedURL := range feedURLs {
			articles[feedURL] = &articleInfo{err: err}
		}
		return articles
	}

	urlsToFetch := make([]string, 0)
	for _, feedURL := range feedURLs {
		if a, ok := existing[feedURL]; ok && a != nil {
			articles[feedURL] = &articleInfo{id: a.ID, title: a.Title, exists: true}
			continue
		}
		urlsToFetch = append(urlsToFetch, feedURL)
		articles[feedURL] = &articleInfo{exists: false}
	}

	if len(urlsToFetch) == 0 {
		return articles
	}

	logger.Logger.InfoContext(ctx, "fetching articles from web", "url_count", len(urlsToFetch))
	fetchResults := u.fetcher.FetchMultiple(ctx, urlsToFetch)

	for urlStr, result := range fetchResults {
		info := articles[urlStr]
		if info == nil {
			continue
		}
		if result.Error != nil {
			info.err = result.Error
			continue
		}

		articleID, err := u.repo.SaveArticle(ctx, urlStr, result.Title, result.Content)
		if err != nil {
			info.err = err
			continue
		}

		info.id = articleID
		info.title = result.Title
		info.exists = true
	}

	return articles
}

// cleanSummaryContent removes markdown code blocks, repetitive patterns, and
// other LLM output anomalies from summary content before it reaches
// clients. The single surviving copy of logic previously duplicated across
// rest/utils.go and rest/rest_feeds/utils.go.
func cleanSummaryContent(summary string) string {
	if summary == "" {
		return ""
	}

	cleaned := summary

	// Remove markdown code blocks (```...```).
	cleaned = codeBlockRegex.ReplaceAllString(cleaned, "")
	// Remove standalone triple backticks.
	cleaned = backtickRegex.ReplaceAllString(cleaned, "")
	// Remove any remaining backticks.
	cleaned = strings.ReplaceAll(cleaned, "`", "")

	// Remove excessive whitespace.
	cleaned = whitespaceRegex.ReplaceAllString(cleaned, " ")
	// Remove excessive newlines.
	cleaned = newlineRegex.ReplaceAllString(cleaned, "\n\n")

	return strings.TrimSpace(cleaned)
}

var (
	codeBlockRegex  = regexp.MustCompile("(?s)```[^`]*```")
	backtickRegex   = regexp.MustCompile("```+")
	whitespaceRegex = regexp.MustCompile(`[ \t]+`)
	newlineRegex    = regexp.MustCompile(`\n{3,}`)
)
