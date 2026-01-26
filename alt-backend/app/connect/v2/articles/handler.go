// Package articles implements the ArticleService Connect-RPC handlers.
package articles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"connectrpc.com/connect"

	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"

	"alt/config"
	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/di"
	"alt/domain"
	"alt/rest"
	"alt/usecase/archive_article_usecase"
)

// Handler implements the ArticleService Connect-RPC service.
type Handler struct {
	container *di.ApplicationComponents
	logger    *slog.Logger
	cfg       *config.Config
}

// NewHandler creates a new Article service handler.
func NewHandler(container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		container: container,
		logger:    logger,
		cfg:       cfg,
	}
}

// Verify interface implementation at compile time.
var _ articlesv2connect.ArticleServiceHandler = (*Handler)(nil)

// FetchArticleContent fetches and extracts compliant article content.
// Replaces GET /v1/articles/fetch/content
func (h *Handler) FetchArticleContent(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleContentRequest],
) (*connect.Response[articlesv2.FetchArticleContentResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate URL
	if req.Msg.Url == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("url is required"))
	}

	parsedURL, err := url.Parse(req.Msg.Url)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid URL format: %w", err))
	}

	// Check for allowed URLs (SSRF protection)
	if err := rest.IsAllowedURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("URL not allowed: %w", err))
	}

	// Call usecase
	content, articleID, err := h.container.ArticleUsecase.FetchCompliantArticle(ctx, parsedURL, *user)
	if err != nil {
		var complianceErr *domain.ComplianceError
		if errors.As(err, &complianceErr) {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("%s", complianceErr.Message))
		}

		if errors.Is(err, context.DeadlineExceeded) {
			return nil, connect.NewError(connect.CodeDeadlineExceeded,
				fmt.Errorf("request timeout"))
		}

		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "FetchArticleContent")
	}

	// Content is already sanitized by usecase (ExtractArticleHTML)
	return connect.NewResponse(&articlesv2.FetchArticleContentResponse{
		Url:       parsedURL.String(),
		Content:   content,
		ArticleId: articleID,
	}), nil
}

// ArchiveArticle archives an article for later reading.
// Replaces POST /v1/articles/archive
func (h *Handler) ArchiveArticle(
	ctx context.Context,
	req *connect.Request[articlesv2.ArchiveArticleRequest],
) (*connect.Response[articlesv2.ArchiveArticleResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate URL
	if req.Msg.FeedUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_url is required"))
	}

	parsedURL, err := url.Parse(req.Msg.FeedUrl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid URL format: %w", err))
	}

	// Check for allowed URLs (SSRF protection)
	if err := rest.IsAllowedURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("URL not allowed: %w", err))
	}

	// Prepare input
	input := archive_article_usecase.ArchiveArticleInput{
		URL:   parsedURL.String(),
		Title: "",
	}
	if req.Msg.Title != nil {
		input.Title = *req.Msg.Title
	}

	// Call usecase
	if err := h.container.ArchiveArticleUsecase.Execute(ctx, input); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "ArchiveArticle")
	}

	return connect.NewResponse(&articlesv2.ArchiveArticleResponse{
		Message: "article archived",
	}), nil
}

// FetchArticlesCursor fetches articles with cursor-based pagination.
// Replaces GET /v1/articles/fetch/cursor
func (h *Handler) FetchArticlesCursor(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticlesCursorRequest],
) (*connect.Response[articlesv2.FetchArticlesCursorResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20 // default
	}
	if limit > 100 {
		limit = 100
	}

	// Parse cursor if provided
	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	// Call usecase (request limit+1 to determine hasMore)
	articles, err := h.container.FetchArticlesCursorUsecase.Execute(ctx, cursor, limit+1)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "FetchArticlesCursor")
	}

	// Determine hasMore and trim result
	hasMore := len(articles) > limit
	if hasMore {
		articles = articles[:limit]
	}

	// Convert to proto
	protoArticles := convertArticlesToProto(articles)

	// Derive next cursor
	var nextCursor *string
	if hasMore && len(articles) > 0 {
		lastArticle := articles[len(articles)-1]
		cursorStr := lastArticle.PublishedAt.Format(time.RFC3339)
		nextCursor = &cursorStr
	}

	return connect.NewResponse(&articlesv2.FetchArticlesCursorResponse{
		Data:       protoArticles,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

// convertArticlesToProto converts domain articles to proto format.
func convertArticlesToProto(articles []*domain.Article) []*articlesv2.ArticleItem {
	result := make([]*articlesv2.ArticleItem, 0, len(articles))
	for _, article := range articles {
		result = append(result, &articlesv2.ArticleItem{
			Id:          article.ID.String(),
			Title:       article.Title,
			Url:         article.URL,
			Content:     article.Content,
			PublishedAt: article.PublishedAt.Format(time.RFC3339),
			Tags:        article.Tags,
		})
	}
	return result
}

// FetchArticleSummary fetches article summaries for multiple URLs.
// Replaces POST /v1/articles/summary
// Priority: 1) AI-generated summaries from article_summaries table
//
//	2) Inoreader feed excerpts from inoreader_summaries table
func (h *Handler) FetchArticleSummary(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleSummaryRequest],
) (*connect.Response[articlesv2.FetchArticleSummaryResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feedUrls := req.Msg.FeedUrls

	// Validation
	if len(feedUrls) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_urls cannot be empty"))
	}
	if len(feedUrls) > 50 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("maximum 50 URLs allowed"))
	}

	items := make([]*articlesv2.ArticleSummaryItem, 0, len(feedUrls))

	// First, try to get AI-generated summaries from article_summaries table
	// Only if AltDBRepository is available (may be nil in tests)
	if h.container.AltDBRepository != nil {
		for _, feedURL := range feedUrls {
			parsedURL, parseErr := url.Parse(feedURL)
			if parseErr != nil {
				h.logger.WarnContext(ctx, "Failed to parse URL for AI summary lookup",
					"url", feedURL,
					"error", parseErr)
				continue
			}

			// Check for allowed URLs (SSRF protection)
			if ssrfErr := rest.IsAllowedURL(parsedURL); ssrfErr != nil {
				h.logger.WarnContext(ctx, "URL not allowed for AI summary lookup",
					"url", feedURL,
					"error", ssrfErr)
				continue
			}

			aiSummary, aiErr := h.container.AltDBRepository.FetchFeedSummary(ctx, parsedURL)
			if aiErr == nil && aiSummary != nil && aiSummary.Summary != "" {
				// Found AI-generated summary
				items = append(items, &articlesv2.ArticleSummaryItem{
					Title:       "AI Summary",
					Content:     aiSummary.Summary,
					Author:      "",
					PublishedAt: time.Now().Format(time.RFC3339),
					FetchedAt:   time.Now().Format(time.RFC3339),
					SourceId:    "",
				})
			}
		}

		// If we found AI summaries, return them
		if len(items) > 0 {
			return connect.NewResponse(&articlesv2.FetchArticleSummaryResponse{
				MatchedArticles: items,
				TotalMatched:    int32(len(items)),
				RequestedCount:  int32(len(feedUrls)),
			}), nil
		}
	}

	// Fallback: Fetch summaries from inoreader_summaries using existing usecase
	summaries, err := h.container.FetchInoreaderSummaryUsecase.Execute(ctx, feedUrls)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "FetchArticleSummary")
	}

	// Convert to proto response
	for _, s := range summaries {
		author := ""
		if s.Author != nil {
			author = *s.Author
		}

		items = append(items, &articlesv2.ArticleSummaryItem{
			Title:       s.Title,
			Content:     s.Content,
			Author:      author,
			PublishedAt: s.PublishedAt.Format(time.RFC3339),
			FetchedAt:   s.FetchedAt.Format(time.RFC3339),
			SourceId:    s.InoreaderID,
		})
	}

	return connect.NewResponse(&articlesv2.FetchArticleSummaryResponse{
		MatchedArticles: items,
		TotalMatched:    int32(len(items)),
		RequestedCount:  int32(len(feedUrls)),
	}), nil
}
