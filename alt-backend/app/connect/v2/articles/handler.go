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
	"alt/usecase/archive_article_usecase"
	"alt/utils/url_validator"
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
	if err := url_validator.IsAllowedURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("URL not allowed: %w", err))
	}

	// Call usecase
	content, articleID, ogImageURL, err := h.container.ArticleUsecase.FetchCompliantArticle(ctx, parsedURL, *user)
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
	resp := &articlesv2.FetchArticleContentResponse{
		Url:        parsedURL.String(),
		Content:    content,
		ArticleId:  articleID,
		OgImageUrl: ogImageURL,
	}

	return connect.NewResponse(resp), nil
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
	if err := url_validator.IsAllowedURL(parsedURL); err != nil {
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

// FetchArticlesByTag fetches articles by tag (ID or name).
// Replaces GET /v1/articles/by-tag
// ADR-169: tag_name で横断検索、tag_id は後方互換性
func (h *Handler) FetchArticlesByTag(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticlesByTagRequest],
) (*connect.Response[articlesv2.FetchArticlesByTagResponse], error) {
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

	// Request limit+1 to determine hasMore
	var articles []*domain.TagTrailArticle

	// Prefer tag_name (ADR-169 cross-feed discovery), fallback to tag_id
	if req.Msg.TagName != nil && *req.Msg.TagName != "" {
		articles, err = h.container.FetchArticlesByTagUsecase.ExecuteByTagName(ctx, *req.Msg.TagName, cursor, limit+1)
	} else if req.Msg.TagId != nil && *req.Msg.TagId != "" {
		articles, err = h.container.FetchArticlesByTagUsecase.Execute(ctx, *req.Msg.TagId, cursor, limit+1)
	} else {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("either tag_id or tag_name is required"))
	}

	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "FetchArticlesByTag")
	}

	// Determine hasMore and trim result
	hasMore := len(articles) > limit
	if hasMore {
		articles = articles[:limit]
	}

	// Convert to proto
	protoArticles := make([]*articlesv2.TagTrailArticleItem, 0, len(articles))
	for _, article := range articles {
		protoArticles = append(protoArticles, &articlesv2.TagTrailArticleItem{
			Id:          article.ID,
			Title:       article.Title,
			Link:        article.Link,
			PublishedAt: article.PublishedAt.Format(time.RFC3339),
			FeedTitle:   article.FeedTitle,
		})
	}

	// Derive next cursor
	var nextCursor *string
	if hasMore && len(articles) > 0 {
		lastArticle := articles[len(articles)-1]
		cursorStr := lastArticle.PublishedAt.Format(time.RFC3339)
		nextCursor = &cursorStr
	}

	return connect.NewResponse(&articlesv2.FetchArticlesByTagResponse{
		Articles:   protoArticles,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

// FetchArticleTags fetches tags for an article.
// Replaces GET /v1/articles/:id/tags
func (h *Handler) FetchArticleTags(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleTagsRequest],
) (*connect.Response[articlesv2.FetchArticleTagsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	articleID := req.Msg.ArticleId
	if articleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_id is required"))
	}

	tags, err := h.container.FetchArticleTagsUsecase.Execute(ctx, articleID)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "FetchArticleTags")
	}

	// Convert to proto
	protoTags := make([]*articlesv2.ArticleTagItem, 0, len(tags))
	for _, tag := range tags {
		protoTags = append(protoTags, &articlesv2.ArticleTagItem{
			Id:        tag.ID,
			Name:      tag.TagName,
			CreatedAt: tag.CreatedAt.Format(time.RFC3339),
		})
	}

	return connect.NewResponse(&articlesv2.FetchArticleTagsResponse{
		ArticleId: articleID,
		Tags:      protoTags,
	}), nil
}

// FetchRandomFeed fetches a random feed for Tag Trail.
// Replaces GET /v1/rss-feed-link/random
// ADR-173: Includes tags for the feed's latest article (generated on-the-fly if not in DB)
func (h *Handler) FetchRandomFeed(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchRandomFeedRequest],
) (*connect.Response[articlesv2.FetchRandomFeedResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feed, err := h.container.FetchRandomSubscriptionUsecase.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "FetchRandomFeed")
	}

	// Fetch tags for the feed's latest article (ADR-173)
	var protoTags []*articlesv2.ArticleTagItem

	if h.container.FetchLatestArticleUsecase != nil {
		// Get the latest article for this feed
		latestArticle, err := h.container.FetchLatestArticleUsecase.Execute(ctx, feed.ID)
		if err != nil {
			h.logger.WarnContext(ctx, "failed to fetch latest article for feed", "feedID", feed.ID, "error", err)
			// Continue without tags - fail-open
		} else if latestArticle != nil {
			h.logger.InfoContext(ctx, "found latest article for feed", "feedID", feed.ID, "articleID", latestArticle.ID)

			// Use FetchArticleTagsUsecase for on-the-fly generation (ADR-168)
			tags, err := h.container.FetchArticleTagsUsecase.Execute(ctx, latestArticle.ID)
			if err != nil {
				h.logger.WarnContext(ctx, "failed to fetch/generate tags for article", "articleID", latestArticle.ID, "error", err)
				// Continue without tags - fail-open
			} else {
				protoTags = convertTagsToProto(tags)
				h.logger.InfoContext(ctx, "fetched tags for feed's latest article",
					"feedID", feed.ID,
					"articleID", latestArticle.ID,
					"tagCount", len(protoTags))
			}
		} else {
			h.logger.InfoContext(ctx, "no articles found for feed, triggering async fetch", "feedID", feed.ID)

			// Capture user before goroutine (user context will be lost after response)
			user, _ := middleware.GetUserContext(ctx)
			if user != nil {
				userCopy := *user
				feedIDCopy := feed.ID
				feedLinkCopy := feed.Link

				// Async article fetch + tag generation (non-blocking)
				go func() {
					bgCtx := context.Background()
					parsedURL, err := url.Parse(feedLinkCopy)
					if err != nil {
						h.logger.Warn("failed to parse feed link for async fetch", "feedID", feedIDCopy, "error", err)
						return
					}
					if err := url_validator.IsAllowedURL(parsedURL); err != nil {
						h.logger.Warn("feed link not allowed for async fetch", "feedID", feedIDCopy, "error", err)
						return
					}
					if h.container.ArticleUsecase == nil {
						return
					}
					_, newArticleID, _, fetchErr := h.container.ArticleUsecase.FetchCompliantArticle(bgCtx, parsedURL, userCopy)
					if fetchErr != nil {
						h.logger.Warn("async article fetch failed", "feedID", feedIDCopy, "error", fetchErr)
						return
					}
					h.logger.Info("async article fetch succeeded", "feedID", feedIDCopy, "articleID", newArticleID)
					if newArticleID != "" {
						_, tagErr := h.container.FetchArticleTagsUsecase.Execute(bgCtx, newArticleID)
						if tagErr != nil {
							h.logger.Warn("async tag fetch failed", "feedID", feedIDCopy, "articleID", newArticleID, "error", tagErr)
						} else {
							h.logger.Info("async tag fetch succeeded", "feedID", feedIDCopy, "articleID", newArticleID)
						}
					}
				}()
			}
		}
	}

	return connect.NewResponse(&articlesv2.FetchRandomFeedResponse{
		Id:          feed.ID.String(),
		Url:         feed.Link, // Site URL (feeds.link)
		Title:       feed.Title,
		Description: feed.Description,
		Tags:        protoTags,
	}), nil
}

// StreamArticleTags streams real-time tag updates for an article.
// Returns cached tags immediately if available, otherwise triggers on-the-fly generation via mq-hub.
// ADR-168: On-the-fly tag generation for Tag Trail initial feed card.
func (h *Handler) StreamArticleTags(
	ctx context.Context,
	req *connect.Request[articlesv2.StreamArticleTagsRequest],
	stream *connect.ServerStream[articlesv2.ArticleTagEvent],
) error {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	articleID := req.Msg.ArticleId
	if articleID == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_id is required"))
	}

	h.logger.InfoContext(ctx, "starting article tags stream", "articleID", articleID)

	if h.container.StreamArticleTagsUsecase == nil {
		h.logger.WarnContext(ctx, "StreamArticleTagsUsecase not available, returning empty tags", "articleID", articleID)
		return stream.Send(&articlesv2.ArticleTagEvent{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.ArticleTagEvent_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("Tag generation not available"),
		})
	}

	result, err := h.container.StreamArticleTagsUsecase.Execute(ctx, articleID)
	if err != nil {
		h.logger.WarnContext(ctx, "tag resolution failed", "articleID", articleID, "error", err)
		return stream.Send(&articlesv2.ArticleTagEvent{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.ArticleTagEvent_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("Tag generation temporarily unavailable"),
		})
	}

	if len(result.Tags) == 0 {
		h.logger.InfoContext(ctx, "no tags found or generated", "articleID", articleID)
		return stream.Send(&articlesv2.ArticleTagEvent{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.ArticleTagEvent_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("No tags generated"),
		})
	}

	eventType := articlesv2.ArticleTagEvent_EVENT_TYPE_COMPLETED
	if result.IsCached {
		eventType = articlesv2.ArticleTagEvent_EVENT_TYPE_CACHED
	}

	h.logger.InfoContext(ctx, "returning tags", "articleID", articleID, "tagCount", len(result.Tags), "cached", result.IsCached)
	return stream.Send(&articlesv2.ArticleTagEvent{
		ArticleId: articleID,
		Tags:      convertTagsToProto(result.Tags),
		EventType: eventType,
	})
}

// convertTagsToProto converts domain tags to proto format.
func convertTagsToProto(tags []*domain.FeedTag) []*articlesv2.ArticleTagItem {
	result := make([]*articlesv2.ArticleTagItem, 0, len(tags))
	for _, tag := range tags {
		result = append(result, &articlesv2.ArticleTagItem{
			Id:        tag.ID,
			Name:      tag.TagName,
			CreatedAt: tag.CreatedAt.Format(time.RFC3339),
		})
	}
	return result
}

// stringPtr returns a pointer to a string.
func stringPtr(s string) *string {
	return &s
}

// BatchPrefetchImages generates proxy URLs and optionally warms cache for OGP images.
func (h *Handler) BatchPrefetchImages(
	ctx context.Context,
	req *connect.Request[articlesv2.BatchPrefetchImagesRequest],
) (*connect.Response[articlesv2.BatchPrefetchImagesResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	articleIDs := req.Msg.ArticleIds
	if len(articleIDs) == 0 {
		return connect.NewResponse(&articlesv2.BatchPrefetchImagesResponse{}), nil
	}
	if len(articleIDs) > 10 {
		articleIDs = articleIDs[:10]
	}

	if h.container.ImageProxyUsecase == nil {
		return connect.NewResponse(&articlesv2.BatchPrefetchImagesResponse{}), nil
	}

	// Fetch OGP URLs from article_heads
	ogURLs, err := h.container.AltDBRepository.FetchOgImageURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "BatchPrefetchImages")
	}

	// Generate proxy URLs
	proxyURLs := h.container.ImageProxyUsecase.BatchGenerateProxyURLs(ctx, ogURLs)

	// Build response
	images := make([]*articlesv2.ImageProxyInfo, 0, len(proxyURLs))
	for articleID, proxyURL := range proxyURLs {
		images = append(images, &articlesv2.ImageProxyInfo{
			ArticleId: articleID,
			ProxyUrl:  proxyURL,
			IsCached:  false, // We don't check cache status in batch for performance
		})
	}

	// Warm cache for images in background
	for _, ogURL := range ogURLs {
		ogURLCopy := ogURL
		go func() {
			warmCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			h.container.ImageProxyUsecase.WarmCache(warmCtx, ogURLCopy)
		}()
	}

	return connect.NewResponse(&articlesv2.BatchPrefetchImagesResponse{
		Images: images,
	}), nil
}

// FetchArticleSummary fetches article summaries for multiple URLs.
// Replaces POST /v1/articles/summary
// Priority: 1) AI-generated summaries from article_summaries table
//
//  2. Inoreader feed excerpts from inoreader_summaries table
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
	if h.container.FetchArticleSummaryUsecase != nil {
		for _, feedURL := range feedUrls {
			parsedURL, parseErr := url.Parse(feedURL)
			if parseErr != nil {
				h.logger.WarnContext(ctx, "Failed to parse URL for AI summary lookup",
					"url", feedURL,
					"error", parseErr)
				continue
			}

			// Check for allowed URLs (SSRF protection)
			if ssrfErr := url_validator.IsAllowedURL(parsedURL); ssrfErr != nil {
				h.logger.WarnContext(ctx, "URL not allowed for AI summary lookup",
					"url", feedURL,
					"error", ssrfErr)
				continue
			}

			aiSummary, aiErr := h.container.FetchArticleSummaryUsecase.Execute(ctx, parsedURL)
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
