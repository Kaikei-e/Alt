package summarize

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"pre-processor/domain"
	"pre-processor/repository"
	"pre-processor/utils/html_parser"
)

// OnDemandService handles on-demand article summarization.
// It consolidates the common flow shared by REST, ConnectRPC, and queue worker:
//   - Fetch article from DB (if needed)
//   - Extract text from HTML (Zero Trust)
//   - Call API to summarize
//   - Save summary to DB
type OnDemandService struct {
	articleRepo repository.ArticleRepository
	summaryRepo repository.SummaryRepository
	apiRepo     repository.ExternalAPIRepository
	logger      *slog.Logger
}

// NewOnDemandService creates a new on-demand summarization service.
func NewOnDemandService(
	articleRepo repository.ArticleRepository,
	summaryRepo repository.SummaryRepository,
	apiRepo repository.ExternalAPIRepository,
	logger *slog.Logger,
) *OnDemandService {
	return &OnDemandService{
		articleRepo: articleRepo,
		summaryRepo: summaryRepo,
		apiRepo:     apiRepo,
		logger:      logger,
	}
}

// SummarizeRequest represents a request to summarize an article.
type SummarizeRequest struct {
	ArticleID string
	Content   string // If empty, fetched from DB
	Title     string
	Priority  string // "high" (UI) / "low" (batch)
}

// SummarizeResult represents the result of summarization.
type SummarizeResult struct {
	Summary   string
	ArticleID string
}

// ResolvedArticle is the result of resolving an article's content from the request or DB.
type ResolvedArticle struct {
	ArticleID string
	Content   string
	Title     string
	UserID    string
}

// ResolveArticle fetches the article from DB and resolves content, applying Zero Trust text extraction.
// Returns the resolved article data needed for summarization.
func (s *OnDemandService) ResolveArticle(ctx context.Context, req SummarizeRequest) (*ResolvedArticle, error) {
	// Fetch article from DB
	article, err := s.articleRepo.FindByID(ctx, req.ArticleID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch article: %w", err)
	}
	if article == nil {
		return nil, domain.ErrArticleNotFound
	}

	content := req.Content
	title := req.Title

	// If content is empty, use article content from DB
	if content == "" {
		if article.Content == "" {
			return nil, domain.ErrArticleContentEmpty
		}
		content = article.Content
		s.logger.InfoContext(ctx, "using content from DB", "article_id", req.ArticleID)
	}

	if title == "" {
		title = article.Title
	}

	// Zero Trust: Extract text from HTML
	content = extractText(content, s.logger, ctx, req.ArticleID)

	if content == "" {
		return nil, domain.ErrEmptyContent
	}

	return &ResolvedArticle{
		ArticleID: req.ArticleID,
		Content:   content,
		Title:     title,
		UserID:    article.UserID,
	}, nil
}

// Summarize performs the full summarization flow: resolve article, call API, save result.
func (s *OnDemandService) Summarize(ctx context.Context, req SummarizeRequest) (*SummarizeResult, error) {
	resolved, err := s.ResolveArticle(ctx, req)
	if err != nil {
		return nil, err
	}

	// Call summarization API
	article := &domain.Article{
		ID:      resolved.ArticleID,
		Content: resolved.Content,
	}

	summarized, err := s.apiRepo.SummarizeArticle(ctx, article, req.Priority)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	s.logger.InfoContext(ctx, "article summarized successfully", "article_id", resolved.ArticleID)

	// Save summary to DB
	articleTitle := resolved.Title
	if articleTitle == "" {
		articleTitle = "Untitled"
	}

	articleSummary := &domain.ArticleSummary{
		ArticleID:       resolved.ArticleID,
		UserID:          resolved.UserID,
		ArticleTitle:    articleTitle,
		SummaryJapanese: summarized.SummaryJapanese,
	}

	if err := s.summaryRepo.Create(ctx, articleSummary); err != nil {
		s.logger.ErrorContext(ctx, "failed to save summary to database", "error", err, "article_id", resolved.ArticleID)
		// Don't fail - return the summary even if DB save fails
		s.logger.WarnContext(ctx, "continuing despite DB save failure", "article_id", resolved.ArticleID)
	} else {
		s.logger.InfoContext(ctx, "summary saved to database successfully", "article_id", resolved.ArticleID)
	}

	return &SummarizeResult{
		Summary:   summarized.SummaryJapanese,
		ArticleID: resolved.ArticleID,
	}, nil
}

// extractText applies Zero Trust text extraction from potentially HTML content.
func extractText(content string, logger *slog.Logger, ctx context.Context, articleID string) string {
	// First extraction pass
	originalLength := len(content)
	extractedText := html_parser.ExtractArticleText(content)

	if extractedText != "" {
		content = extractedText
		extractedLength := len(extractedText)
		reductionRatio := (1.0 - float64(extractedLength)/float64(originalLength)) * 100.0
		logger.InfoContext(ctx, "text extraction completed",
			"article_id", articleID,
			"original_length", originalLength,
			"extracted_length", extractedLength,
			"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))
	} else {
		logger.WarnContext(ctx, "text extraction returned empty, using original content",
			"article_id", articleID, "original_length", originalLength)
	}

	// Zero Trust re-extraction: if content still contains HTML
	if strings.Contains(content, "<") && strings.Contains(content, ">") {
		logger.WarnContext(ctx, "content still contains HTML, re-extracting",
			"article_id", articleID, "content_length", len(content))
		reExtracted := html_parser.ExtractArticleText(content)
		if reExtracted != "" {
			content = reExtracted
		}
	}

	return content
}
