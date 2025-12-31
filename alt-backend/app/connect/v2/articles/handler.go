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
	"alt/connect/v2/middleware"
	"alt/di"
	"alt/domain"
	"alt/rest"
	"alt/usecase/archive_article_usecase"
	"alt/utils/html_parser"
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
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
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

		h.logger.Error("failed to fetch article", "error", err, "url", req.Msg.Url)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Strip HTML tags from content
	escapedContent := html_parser.StripTags(content)

	return connect.NewResponse(&articlesv2.FetchArticleContentResponse{
		Url:       parsedURL.String(),
		Content:   escapedContent,
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
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
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
		h.logger.Error("failed to archive article", "error", err, "url", req.Msg.FeedUrl)
		return nil, connect.NewError(connect.CodeInternal, err)
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
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
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
		h.logger.Error("failed to fetch articles", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
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
