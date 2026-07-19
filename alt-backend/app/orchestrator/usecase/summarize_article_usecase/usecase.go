// Package summarize_article_usecase orchestrates the "resolve an article,
// then get its AI summary" flow shared by the legacy single-URL summarize
// endpoints (POST /v1/feeds/summarize, /summarize/queue, /summarize/stream,
// GET /summarize/status/:job_id). It replaces handler code that called
// container.AltDBRepository (a driver) and the pre-processor HTTP API
// directly.
package summarize_article_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/fetch_article_port"
	"alt/orchestrator/port/preprocessor_summarize_port"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"context"
	"fmt"
	"io"
	"time"
)

// ArticleRepository is the narrow slice of the article/summary tables this
// usecase needs. It is satisfied structurally by *alt_db.AltDBRepository.
type ArticleRepository interface {
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
	FetchArticleByID(ctx context.Context, articleID string) (*domain.ArticleContent, error)
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
	FetchArticleSummaryByArticleID(ctx context.Context, articleID string) (*domain.FeedSummary, error)
	SaveArticleSummary(ctx context.Context, articleID, userID, articleTitle, summary string) error
}

// summarizeDelay guards against reading the article back from the
// pre-processor's DB connection before the SaveArticle transaction that
// created it has become visible (pull-model summarization). Preserves the
// existing bandaid behavior; not fixed here (tracked separately).
const summarizeDelay = 100 * time.Millisecond

// Usecase resolves articles by URL and generates/queues their AI summaries.
type Usecase struct {
	repo         ArticleRepository
	preprocessor preprocessor_summarize_port.PreProcessorSummarizePort
	fetcher      fetch_article_port.FetchArticlePort
}

// NewUsecase creates a Usecase. repo may be nil to simulate a database
// outage in tests; preprocessor and fetcher are expected to always be wired
// by the composition root.
func NewUsecase(repo ArticleRepository, preprocessor preprocessor_summarize_port.PreProcessorSummarizePort, fetcher fetch_article_port.FetchArticlePort) *Usecase {
	return &Usecase{repo: repo, preprocessor: preprocessor, fetcher: fetcher}
}

// EnsureArticle resolves feedURL to an article ID and title, fetching the
// page from the web and persisting it when it is not already in the
// database.
func (u *Usecase) EnsureArticle(ctx context.Context, feedURL string) (articleID string, title string, existed bool, err error) {
	if u.repo == nil {
		return "", "", false, fmt.Errorf("article repository not available")
	}

	existing, err := u.repo.FetchArticleByURL(ctx, feedURL)
	if err != nil {
		return "", "", false, fmt.Errorf("fetch article by url: %w", err)
	}
	if existing != nil {
		return existing.ID, existing.Title, true, nil
	}

	content, fetchedTitle, err := u.fetchAndExtract(ctx, feedURL)
	if err != nil {
		return "", "", false, fmt.Errorf("fetch article content: %w", err)
	}

	id, err := u.repo.SaveArticle(ctx, feedURL, fetchedTitle, content)
	if err != nil {
		return "", "", false, fmt.Errorf("save article: %w", err)
	}

	return id, fetchedTitle, false, nil
}

// EnsureSummary returns a cached summary for articleID if one exists,
// otherwise it generates one via the pre-processor and persists it.
// A save failure after generation is logged but not returned: the caller
// still receives a usable summary even if persistence fails.
func (u *Usecase) EnsureSummary(ctx context.Context, articleID, userID, title string) (summary string, fromCache bool, err error) {
	if cached, ok := u.GetCachedSummary(ctx, articleID); ok {
		return cached, true, nil
	}

	// Small delay to ensure the DB transaction that created/updated the
	// article is visible to the pre-processor's read (pull model reads
	// content from DB).
	time.Sleep(summarizeDelay)

	summary, err = u.preprocessor.Summarize(ctx, "", articleID, title)
	if err != nil {
		return "", false, fmt.Errorf("summarize article: %w", err)
	}

	if saveErr := u.repo.SaveArticleSummary(ctx, articleID, userID, title, summary); saveErr != nil {
		logger.Logger.ErrorContext(ctx, "failed to save article summary", "error", saveErr, "article_id", articleID)
	} else {
		logger.Logger.InfoContext(ctx, "article summary saved", "article_id", articleID)
	}

	return summary, false, nil
}

// ResolveStreamArticle resolves the article targeted by a streaming
// summarize request, preferring caller-provided title/content over stored
// values. With an articleID it backfills missing content from the database
// (a missing article is not an error — the caller validates content
// afterwards). With only a feedURL it reuses the stored article, or persists
// the caller-provided content, or fetches the page from the web and persists
// that.
func (u *Usecase) ResolveStreamArticle(ctx context.Context, articleID, feedURL, title, content string) (resolvedID, resolvedTitle, resolvedContent string, err error) {
	if u.repo == nil {
		return "", "", "", fmt.Errorf("article repository not available")
	}

	if articleID != "" {
		if content == "" {
			article, err := u.repo.FetchArticleByID(ctx, articleID)
			if err != nil {
				return "", "", "", fmt.Errorf("fetch article by id: %w", err)
			}
			if article != nil {
				logger.Logger.InfoContext(ctx, "Fetched article content from DB", "article_id", articleID, "content_length", len(article.Content))
				content = article.Content
				if title == "" {
					title = article.Title
				}
			} else {
				logger.Logger.WarnContext(ctx, "Article ID provided but not found in DB", "article_id", articleID)
			}
		}
		return articleID, title, content, nil
	}

	existing, err := u.repo.FetchArticleByURL(ctx, feedURL)
	if err != nil {
		return "", "", "", fmt.Errorf("fetch article by url: %w", err)
	}
	if existing != nil {
		if title == "" {
			title = existing.Title
		}
		if content == "" {
			content = existing.Content
		}
		return existing.ID, title, content, nil
	}

	if content != "" {
		if title == "" {
			title = "No Title"
		}
		id, err := u.repo.SaveArticle(ctx, feedURL, title, content)
		if err != nil {
			return "", "", "", fmt.Errorf("save article: %w", err)
		}
		return id, title, content, nil
	}

	fetchedContent, fetchedTitle, err := u.fetchAndExtract(ctx, feedURL)
	if err != nil {
		return "", "", "", fmt.Errorf("fetch article content: %w", err)
	}
	id, err := u.repo.SaveArticle(ctx, feedURL, fetchedTitle, fetchedContent)
	if err != nil {
		return "", "", "", fmt.Errorf("save article: %w", err)
	}
	return id, fetchedTitle, fetchedContent, nil
}

// StreamSummary opens a streaming summarization for articleID via the
// pre-processor. The caller must close the returned stream.
func (u *Usecase) StreamSummary(ctx context.Context, content, articleID, title string) (io.ReadCloser, error) {
	stream, err := u.preprocessor.StreamSummarize(ctx, content, articleID, title)
	if err != nil {
		return nil, fmt.Errorf("stream summarize: %w", err)
	}
	return stream, nil
}

// SaveStreamedSummary persists a summary captured from a completed stream.
func (u *Usecase) SaveStreamedSummary(ctx context.Context, articleID, userID, title, summary string) error {
	if u.repo == nil {
		return fmt.Errorf("article repository not available")
	}
	if err := u.repo.SaveArticleSummary(ctx, articleID, userID, title, summary); err != nil {
		return fmt.Errorf("save article summary: %w", err)
	}
	return nil
}

// GetCachedSummary returns a previously generated, non-empty summary for
// articleID, if any. A lookup error is treated the same as "not found" —
// matches the legacy handlers' behavior of falling back to generation.
func (u *Usecase) GetCachedSummary(ctx context.Context, articleID string) (string, bool) {
	if u.repo == nil {
		return "", false
	}
	existing, err := u.repo.FetchArticleSummaryByArticleID(ctx, articleID)
	if err != nil || existing == nil || existing.Summary == "" {
		return "", false
	}
	return existing.Summary, true
}

// QueueSummary submits articleID for asynchronous summarization. Callers
// should check GetCachedSummary first — this always queues a new job.
func (u *Usecase) QueueSummary(ctx context.Context, articleID, title string) (string, error) {
	// Small delay to ensure the DB transaction that created/updated the
	// article is visible to the pre-processor's read (pull model reads
	// content from DB).
	time.Sleep(summarizeDelay)

	jobID, err := u.preprocessor.QueueSummarize(ctx, articleID, title)
	if err != nil {
		return "", fmt.Errorf("queue summarize: %w", err)
	}
	return jobID, nil
}

// SummaryStatus checks the status of a previously queued summarization job.
func (u *Usecase) SummaryStatus(ctx context.Context, jobID string) (*preprocessor_summarize_port.SummarizeStatus, error) {
	status, err := u.preprocessor.GetSummarizeStatus(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get summarize status: %w", err)
	}
	return status, nil
}

// fetchAndExtract fetches urlStr via the SSRF-safe article fetch port and
// extracts its title and readable text. Falls back to the raw HTML when
// text extraction yields nothing (matches legacy fetchArticleContent
// behavior).
func (u *Usecase) fetchAndExtract(ctx context.Context, urlStr string) (content string, title string, err error) {
	contentPtr, err := u.fetcher.FetchArticleContents(ctx, urlStr)
	if err != nil {
		return "", "", err
	}

	htmlContent := ""
	if contentPtr != nil {
		htmlContent = *contentPtr
	}

	extractedTitle := html_parser.ExtractTitle(htmlContent)
	extractedText := html_parser.ExtractArticleText(htmlContent)
	if extractedText == "" {
		logger.Logger.WarnContext(ctx, "failed to extract article text from HTML, falling back to raw HTML",
			"url", urlStr, "html_size_bytes", len(htmlContent))
		return htmlContent, extractedTitle, nil
	}

	return extractedText, extractedTitle, nil
}
