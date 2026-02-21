// Package feeds implements the FeedService Connect-RPC handlers.
package feeds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"

	"alt/config"
	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/di"
	"alt/domain"
	"alt/utils/html_parser"
	"alt/utils/url_validator"
)

// Handler implements the FeedService Connect-RPC service.
type Handler struct {
	container *di.ApplicationComponents
	logger    *slog.Logger
	cfg       *config.Config
}

// NewHandler creates a new Feed service handler.
func NewHandler(container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		container: container,
		logger:    logger,
		cfg:       cfg,
	}
}

// Verify interface implementation at compile time.
var _ feedsv2connect.FeedServiceHandler = (*Handler)(nil)

// GetFeedStats returns basic feed statistics (feed count, summarized count).
func (h *Handler) GetFeedStats(
	ctx context.Context,
	req *connect.Request[feedsv2.GetFeedStatsRequest],
) (*connect.Response[feedsv2.GetFeedStatsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feedCount, err := h.container.FeedAmountUsecase.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetFeedStats.FeedAmount")
	}

	summarizedCount, err := h.container.SummarizedArticlesCountUsecase.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetFeedStats.SummarizedCount")
	}

	return connect.NewResponse(&feedsv2.GetFeedStatsResponse{
		FeedAmount:           int64(feedCount),
		SummarizedFeedAmount: int64(summarizedCount),
	}), nil
}

// GetDetailedFeedStats returns detailed feed statistics.
func (h *Handler) GetDetailedFeedStats(
	ctx context.Context,
	req *connect.Request[feedsv2.GetDetailedFeedStatsRequest],
) (*connect.Response[feedsv2.GetDetailedFeedStatsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feedCount, err := h.container.FeedAmountUsecase.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetDetailedFeedStats.FeedAmount")
	}

	articleCount, err := h.container.TotalArticlesCountUsecase.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetDetailedFeedStats.ArticleCount")
	}

	unsummarizedCount, err := h.container.UnsummarizedArticlesCountUsecase.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetDetailedFeedStats.UnsummarizedCount")
	}

	return connect.NewResponse(&feedsv2.GetDetailedFeedStatsResponse{
		FeedAmount:             int64(feedCount),
		ArticleAmount:          int64(articleCount),
		UnsummarizedFeedAmount: int64(unsummarizedCount),
	}), nil
}

// GetUnreadCount returns the count of unread articles for today.
func (h *Handler) GetUnreadCount(
	ctx context.Context,
	req *connect.Request[feedsv2.GetUnreadCountRequest],
) (*connect.Response[feedsv2.GetUnreadCountResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Default to start of today (00:00:00 UTC)
	now := time.Now().UTC()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	count, err := h.container.TodayUnreadArticlesCountUsecase.Execute(ctx, since)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetUnreadCount")
	}

	return connect.NewResponse(&feedsv2.GetUnreadCountResponse{
		Count: int64(count),
	}), nil
}

// StreamFeedStats streams real-time feed statistics updates.
// Replaces the SSE endpoint /v1/sse/feeds/stats with Connect-RPC Server Streaming.
func (h *Handler) StreamFeedStats(
	ctx context.Context,
	req *connect.Request[feedsv2.StreamFeedStatsRequest],
	stream *connect.ServerStream[feedsv2.StreamFeedStatsResponse],
) error {
	// Authentication check
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Get update intervals from config
	updateInterval := h.cfg.Server.SSEInterval
	if updateInterval == 0 {
		updateInterval = 5 * time.Second
	}
	heartbeatInterval := 10 * time.Second

	h.logger.InfoContext(ctx, "starting feed stats stream",
		"update_interval", updateInterval,
		"heartbeat_interval", heartbeatInterval)

	// Create tickers
	updateTicker := time.NewTicker(updateInterval)
	defer updateTicker.Stop()

	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	// Send initial data immediately
	if err := h.sendStatsUpdate(ctx, stream, false); err != nil {
		h.logger.ErrorContext(ctx, "failed to send initial stats", "error", err)
		return err
	}

	// Stream loop
	for {
		select {
		case <-ctx.Done():
			// Client disconnected or context cancelled
			h.logger.InfoContext(ctx, "feed stats stream cancelled", "reason", ctx.Err())
			return nil

		case <-updateTicker.C:
			// Send periodic data update
			if err := h.sendStatsUpdate(ctx, stream, false); err != nil {
				h.logger.ErrorContext(ctx, "failed to send stats update", "error", err)
				return err
			}

		case <-heartbeatTicker.C:
			// Send heartbeat to keep connection alive
			if err := h.sendStatsUpdate(ctx, stream, true); err != nil {
				h.logger.ErrorContext(ctx, "failed to send heartbeat", "error", err)
				return err
			}
		}
	}
}

// sendStatsUpdate sends a stats update or heartbeat message to the stream.
func (h *Handler) sendStatsUpdate(
	ctx context.Context,
	stream *connect.ServerStream[feedsv2.StreamFeedStatsResponse],
	isHeartbeat bool,
) error {
	resp := &feedsv2.StreamFeedStatsResponse{
		Metadata: &feedsv2.ResponseMetadata{
			Timestamp:   time.Now().Unix(),
			IsHeartbeat: isHeartbeat,
		},
	}

	if !isHeartbeat {
		// Fetch actual stats from usecases
		feedCount, err := h.container.FeedAmountUsecase.Execute(ctx)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to get feed count", "error", err)
			return err
		}

		unsummarized, err := h.container.UnsummarizedArticlesCountUsecase.Execute(ctx)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to get unsummarized count", "error", err)
			return err
		}

		totalArticles, err := h.container.TotalArticlesCountUsecase.Execute(ctx)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to get total articles", "error", err)
			return err
		}

		resp.FeedAmount = int64(feedCount)
		resp.UnsummarizedFeedAmount = int64(unsummarized)
		resp.TotalArticles = int64(totalArticles)
	}

	return stream.Send(resp)
}

// =============================================================================
// Feed List RPCs (Phase 2)
// =============================================================================

// GetUnreadFeeds returns unread feeds with cursor-based pagination.
// Replaces GET /v1/feeds/fetch/cursor
func (h *Handler) GetUnreadFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.GetUnreadFeedsRequest],
) (*connect.Response[feedsv2.GetUnreadFeedsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20 // default
		if req.Msg.View != nil && *req.Msg.View == "swipe" {
			limit = 1
		}
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

	// Parse exclude_feed_link_id if provided
	var excludeFeedLinkID *uuid.UUID
	if req.Msg.ExcludeFeedLinkId != nil && *req.Msg.ExcludeFeedLinkId != "" {
		parsed, err := uuid.Parse(*req.Msg.ExcludeFeedLinkId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid exclude_feed_link_id: %w", err))
		}
		excludeFeedLinkID = &parsed
	}

	// Call usecase
	feeds, hasMore, err := h.container.FetchUnreadFeedsListCursorUsecase.Execute(ctx, cursor, limit, excludeFeedLinkID)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetUnreadFeeds")
	}

	return connect.NewResponse(&feedsv2.GetUnreadFeedsResponse{
		Data:       convertFeedsToProto(feeds),
		NextCursor: deriveNextCursor(feeds, hasMore),
		HasMore:    hasMore,
	}), nil
}

// GetAllFeeds returns all feeds (read + unread) with cursor-based pagination.
func (h *Handler) GetAllFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.GetAllFeedsRequest],
) (*connect.Response[feedsv2.GetAllFeedsResponse], error) {
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

	// Parse exclude_feed_link_id if provided
	var excludeFeedLinkID *uuid.UUID
	if req.Msg.ExcludeFeedLinkId != nil && *req.Msg.ExcludeFeedLinkId != "" {
		parsed, err := uuid.Parse(*req.Msg.ExcludeFeedLinkId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid exclude_feed_link_id: %w", err))
		}
		excludeFeedLinkID = &parsed
	}

	// Call usecase (all feeds, no read status filter)
	feeds, err := h.container.FetchFeedsListCursorUsecase.Execute(ctx, cursor, limit, excludeFeedLinkID)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetAllFeeds")
	}

	// Determine hasMore based on result count vs requested limit
	hasMore := len(feeds) >= limit

	return connect.NewResponse(&feedsv2.GetAllFeedsResponse{
		Data:       convertFeedsToProto(feeds),
		NextCursor: deriveNextCursor(feeds, hasMore),
		HasMore:    hasMore,
	}), nil
}

// GetReadFeeds returns read/viewed feeds with cursor-based pagination.
// Replaces GET /v1/feeds/fetch/viewed/cursor
func (h *Handler) GetReadFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.GetReadFeedsRequest],
) (*connect.Response[feedsv2.GetReadFeedsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 32 // default for read feeds
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

	// Call usecase
	feeds, err := h.container.FetchReadFeedsListCursorUsecase.Execute(ctx, cursor, limit)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetReadFeeds")
	}

	// Determine hasMore based on result count vs requested limit
	hasMore := len(feeds) >= limit

	return connect.NewResponse(&feedsv2.GetReadFeedsResponse{
		Data:       convertFeedsToProto(feeds),
		NextCursor: deriveNextCursor(feeds, hasMore),
		HasMore:    hasMore,
	}), nil
}

// GetFavoriteFeeds returns favorite feeds with cursor-based pagination.
// Replaces GET /v1/feeds/fetch/favorites/cursor
func (h *Handler) GetFavoriteFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.GetFavoriteFeedsRequest],
) (*connect.Response[feedsv2.GetFavoriteFeedsResponse], error) {
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

	// Call usecase
	feeds, err := h.container.FetchFavoriteFeedsListCursorUsecase.Execute(ctx, cursor, limit)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetFavoriteFeeds")
	}

	// Determine hasMore based on result count vs requested limit
	hasMore := len(feeds) >= limit

	return connect.NewResponse(&feedsv2.GetFavoriteFeedsResponse{
		Data:       convertFeedsToProto(feeds),
		NextCursor: deriveNextCursor(feeds, hasMore),
		HasMore:    hasMore,
	}), nil
}

// =============================================================================
// Feed Search RPC (Phase 3)
// =============================================================================

// SearchFeeds searches for feeds by query with offset-based pagination.
// Replaces POST /v1/feeds/search
func (h *Handler) SearchFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.SearchFeedsRequest],
) (*connect.Response[feedsv2.SearchFeedsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate query
	if req.Msg.Query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("query must not be empty"))
	}

	// Parse pagination params (offset-based)
	offset := 0
	if req.Msg.Cursor != nil {
		offset = int(*req.Msg.Cursor)
		if offset < 0 {
			offset = 0
		}
	}

	limit := 20
	if req.Msg.Limit != nil {
		limit = int(*req.Msg.Limit)
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
	}

	// Call usecase with pagination
	results, hasMore, err := h.container.FeedSearchUsecase.ExecuteWithPagination(
		ctx, req.Msg.Query, offset, limit)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "SearchFeeds")
	}

	// Compute next cursor
	var nextCursor *int32
	if hasMore {
		next := int32(offset + len(results))
		nextCursor = &next
	}

	return connect.NewResponse(&feedsv2.SearchFeedsResponse{
		Data:       convertFeedsToProto(results),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

// =============================================================================
// Streaming Summarize RPC (Phase 6)
// =============================================================================

// StreamSummarize streams article summarization in real-time.
// Replaces POST /v1/feeds/summarize/stream (SSE)
func (h *Handler) StreamSummarize(
	ctx context.Context,
	req *connect.Request[feedsv2.StreamSummarizeRequest],
	stream *connect.ServerStream[feedsv2.StreamSummarizeResponse],
) error {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate request: feed_url or article_id is required
	feedURL := ""
	if req.Msg.FeedUrl != nil {
		feedURL = *req.Msg.FeedUrl
	}
	articleID := ""
	if req.Msg.ArticleId != nil {
		articleID = *req.Msg.ArticleId
	}

	if feedURL == "" && articleID == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_url or article_id is required"))
	}

	// Get optional content and title
	content := ""
	if req.Msg.Content != nil {
		content = *req.Msg.Content
	}
	title := ""
	if req.Msg.Title != nil {
		title = *req.Msg.Title
	}

	// Resolve article ID and content
	resolvedArticleID, resolvedTitle, resolvedContent, err := h.resolveArticle(ctx, feedURL, articleID, content, title)
	if err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamSummarize.ResolveArticle")
	}

	if resolvedContent == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("content cannot be empty for summarization"))
	}

	// Check cache for existing summary
	existingSummary, err := h.container.AltDBRepository.FetchArticleSummaryByArticleID(ctx, resolvedArticleID)
	if err == nil && existingSummary != nil && existingSummary.Summary != "" {
		h.logger.InfoContext(ctx, "returning cached summary", "article_id", resolvedArticleID)
		// Return cached summary immediately
		return stream.Send(&feedsv2.StreamSummarizeResponse{
			Chunk:       "",
			IsFinal:     true,
			ArticleId:   resolvedArticleID,
			IsCached:    true,
			FullSummary: &existingSummary.Summary,
		})
	}

	h.logger.InfoContext(ctx, "starting stream summarization",
		"article_id", resolvedArticleID,
		"content_length", len(resolvedContent))

	// Stream from pre-processor
	preProcessorStream, err := h.streamPreProcessorSummarize(ctx, resolvedContent, resolvedArticleID, resolvedTitle)
	if err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamSummarize.StartStream")
	}
	defer func() {
		if closeErr := preProcessorStream.Close(); closeErr != nil {
			h.logger.DebugContext(ctx, "failed to close pre-processor stream", "error", closeErr)
		}
	}()

	// Stream chunks to client and capture full summary
	fullSummary, err := h.streamAndCapture(ctx, stream, preProcessorStream, resolvedArticleID)
	if err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamSummarize.Streaming")
	}

	// Save summary to database
	if fullSummary != "" && resolvedArticleID != "" {
		if err := h.container.AltDBRepository.SaveArticleSummary(ctx, resolvedArticleID, userCtx.UserID.String(), resolvedTitle, fullSummary); err != nil {
			h.logger.ErrorContext(ctx, "failed to save summary", "error", err, "article_id", resolvedArticleID)
			// Don't return error, streaming was successful
		} else {
			h.logger.InfoContext(ctx, "summary saved", "article_id", resolvedArticleID, "summary_length", len(fullSummary))
		}
	}

	// Send final message
	return stream.Send(&feedsv2.StreamSummarizeResponse{
		Chunk:       "",
		IsFinal:     true,
		ArticleId:   resolvedArticleID,
		IsCached:    false,
		FullSummary: &fullSummary,
	})
}

// =============================================================================
// StreamSummarize Helper Methods
// =============================================================================

// resolveArticle resolves the article ID and content from the request parameters.
// It handles the following cases:
// 1. article_id provided -> always fetch from DB (DB content is authoritative)
// 2. article_id provided but DB content empty -> fallback to request content
// 3. feed_url provided -> check DB or fetch from URL
func (h *Handler) resolveArticle(ctx context.Context, feedURL, articleID, content, title string) (string, string, string, error) {
	// Case 1 & 2: article_id provided - always fetch from DB first (DB content is authoritative)
	if articleID != "" {
		article, err := h.container.AltDBRepository.FetchArticleByID(ctx, articleID)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to fetch article by ID: %w", err)
		}
		if article != nil && article.Content != "" {
			// DB has content - use it (authoritative source)
			if title == "" {
				title = article.Title
			}
			return articleID, title, article.Content, nil
		}
		// DB content is empty - fallback to provided content
		if content != "" {
			return articleID, title, content, nil
		}
		// Neither DB nor request has content
		return "", "", "", fmt.Errorf("article not found or content is empty")
	}

	// Case 3: feed_url provided
	if feedURL == "" {
		return "", "", "", fmt.Errorf("feed_url or article_id is required")
	}

	// Check if article exists in DB
	existingArticle, err := h.container.AltDBRepository.FetchArticleByURL(ctx, feedURL)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch article by URL: %w", err)
	}

	if existingArticle != nil {
		resolvedTitle := title
		if resolvedTitle == "" {
			resolvedTitle = existingArticle.Title
		}
		resolvedContent := content
		if resolvedContent == "" {
			resolvedContent = existingArticle.Content
		}
		return existingArticle.ID, resolvedTitle, resolvedContent, nil
	}

	// Article doesn't exist, need to fetch or use provided content
	if content != "" {
		// Use provided content and save
		if title == "" {
			title = "No Title"
		}
		newArticleID, err := h.container.AltDBRepository.SaveArticle(ctx, feedURL, title, content)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to save article: %w", err)
		}
		return newArticleID, title, content, nil
	}

	// Fetch content from URL
	fetchedContent, fetchedTitle, err := h.fetchArticleContent(ctx, feedURL)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch article content: %w", err)
	}

	if title == "" {
		title = fetchedTitle
	}

	// Save the article
	newArticleID, err := h.container.AltDBRepository.SaveArticle(ctx, feedURL, title, fetchedContent)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to save article: %w", err)
	}

	return newArticleID, title, fetchedContent, nil
}

// fetchArticleContent fetches and extracts content from a URL.
func (h *Handler) fetchArticleContent(ctx context.Context, urlStr string) (string, string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	// SSRF protection
	if err := url_validator.IsAllowedURL(parsedURL); err != nil {
		return "", "", fmt.Errorf("URL not allowed: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AltBot/1.0; +http://alt.com/bot)")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB limit
	if err != nil {
		return "", "", fmt.Errorf("failed to read body: %w", err)
	}

	htmlContent := string(bodyBytes)
	title := html_parser.ExtractTitle(htmlContent)
	extractedText := html_parser.ExtractArticleText(htmlContent)

	if extractedText == "" {
		h.logger.WarnContext(ctx, "failed to extract article text, using raw HTML", "url", urlStr)
		return htmlContent, title, nil
	}

	return extractedText, title, nil
}

// streamPreProcessorSummarize calls the pre-processor streaming API.
// It creates an independent context to prevent client disconnection from cancelling the stream,
// but also monitors the original context to propagate cancellation when client disconnects.
func (h *Handler) streamPreProcessorSummarize(ctx context.Context, content, articleID, title string) (io.ReadCloser, error) {
	if articleID == "" {
		return nil, fmt.Errorf("article_id is required")
	}

	requestBody := map[string]string{
		"content":    content,
		"article_id": articleID,
		"title":      title,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{
		Timeout: 0, // No timeout for streaming - context handles cancellation
	}

	// Create an independent context for the streaming request.
	// This prevents client disconnection (e.g., butterfly-facade timeout) from cancelling
	// the pre-processor stream mid-generation. Use 10-minute timeout for long articles.
	streamCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	// Monitor client context in a separate goroutine.
	// When client disconnects, cancel the pre-processor request to prevent "zombie requests"
	// that hold locks in pre-processor's processingArticles map.
	go func() {
		select {
		case <-ctx.Done():
			// Client disconnected - propagate cancellation to pre-processor
			h.logger.InfoContext(ctx, "client disconnected, cancelling pre-processor stream",
				"article_id", articleID,
				"reason", ctx.Err())
			cancel()
		case <-streamCtx.Done():
			// Stream completed normally or timed out - nothing to do
		}
	}()

	apiURL := fmt.Sprintf("%s/api/v1/summarize/stream", h.cfg.PreProcessor.URL)
	req, err := http.NewRequestWithContext(streamCtx, http.MethodPost, apiURL, bytes.NewReader(jsonData))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		cancel()
		// Check if original context was cancelled
		if ctx.Err() != nil {
			h.logger.WarnContext(ctx, "pre-processor stream failed due to client context cancellation",
				"article_id", articleID,
				"error", err)
			return nil, fmt.Errorf("client disconnected during stream setup: %w", ctx.Err())
		}
		return nil, fmt.Errorf("failed to call pre-processor stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		cancel() // Cancel context on error
		bodyBytes, _ := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - error response has been read
			_ = closeErr
		}
		return nil, fmt.Errorf("pre-processor returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	h.logger.InfoContext(ctx, "pre-processor stream response received",
		"article_id", articleID,
		"status", resp.Status,
		"content_type", resp.Header.Get("Content-Type"))

	// Wrap the response body to cancel the context when closed
	return &streamReaderWithCancel{
		ReadCloser: resp.Body,
		cancel:     cancel,
	}, nil
}

// streamReaderWithCancel wraps an io.ReadCloser and cancels the context when closed.
type streamReaderWithCancel struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (s *streamReaderWithCancel) Close() error {
	s.cancel()
	return s.ReadCloser.Close()
}

// streamAndCapture streams data from pre-processor to Connect stream and captures the full summary.
// It parses SSE events and sends only the data content to the client.
func (h *Handler) streamAndCapture(
	ctx context.Context,
	stream *connect.ServerStream[feedsv2.StreamSummarizeResponse],
	preProcessorStream io.Reader,
	articleID string,
) (string, error) {
	var summaryBuf strings.Builder
	var sseBuf strings.Builder
	responseBuf := make([]byte, 256)
	bytesWritten := 0

	for {
		select {
		case <-ctx.Done():
			h.logger.InfoContext(ctx, "stream cancelled", "article_id", articleID)
			return summaryBuf.String(), ctx.Err()
		default:
		}

		n, err := preProcessorStream.Read(responseBuf)
		if n > 0 {
			bytesWritten += n
			sseBuf.Write(responseBuf[:n])

			// Process complete SSE events (separated by double newline)
			for {
				sseData := sseBuf.String()
				splitIdx := strings.Index(sseData, "\n\n")
				if splitIdx == -1 {
					break // No complete event yet
				}

				// Extract the complete event
				eventStr := sseData[:splitIdx]
				sseBuf.Reset()
				sseBuf.WriteString(sseData[splitIdx+2:])

				// Parse the SSE event and extract data content
				dataContent := extractSSEData(eventStr)
				if dataContent != "" {
					summaryBuf.WriteString(dataContent)

					// Send parsed content to client
					if sendErr := stream.Send(&feedsv2.StreamSummarizeResponse{
						Chunk:     dataContent,
						IsFinal:   false,
						ArticleId: articleID,
						IsCached:  false,
					}); sendErr != nil {
						h.logger.ErrorContext(ctx, "failed to send chunk", "error", sendErr, "article_id", articleID)
						return "", sendErr
					}
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				// Process any remaining data in buffer
				if sseBuf.Len() > 0 {
					dataContent := extractSSEData(sseBuf.String())
					if dataContent != "" {
						summaryBuf.WriteString(dataContent)
						_ = stream.Send(&feedsv2.StreamSummarizeResponse{
							Chunk:     dataContent,
							IsFinal:   false,
							ArticleId: articleID,
							IsCached:  false,
						})
					}
				}
				h.logger.InfoContext(ctx, "stream completed", "article_id", articleID, "bytes_written", bytesWritten)
				break
			}
			h.logger.ErrorContext(ctx, "failed to read from stream", "error", err, "article_id", articleID)
			return "", err
		}
	}

	return summaryBuf.String(), nil
}

// extractSSEData extracts the data content from an SSE event string.
// It attempts to JSON-decode the data content to handle escaped Unicode characters.
func extractSSEData(eventStr string) string {
	var result strings.Builder
	lines := strings.Split(eventStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			dataContent := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			// Try to JSON-decode the content to handle escaped Unicode
			var decoded string
			if err := json.Unmarshal([]byte(dataContent), &decoded); err == nil {
				result.WriteString(decoded)
			} else {
				// Fallback: use raw content if not valid JSON
				result.WriteString(dataContent)
			}
		}
	}
	return result.String()
}

// =============================================================================
// Mark As Read RPC (Phase 7)
// =============================================================================

// MarkAsRead marks an article as read by its URL.
// Replaces POST /v1/feeds/read
func (h *Handler) MarkAsRead(
	ctx context.Context,
	req *connect.Request[feedsv2.MarkAsReadRequest],
) (*connect.Response[feedsv2.MarkAsReadResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate article_url
	if req.Msg.ArticleUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_url is required"))
	}

	// Parse URL
	articleURL, err := url.Parse(req.Msg.ArticleUrl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid article_url: %w", err))
	}

	// Execute usecase
	if err := h.container.ArticlesReadingStatusUsecase.Execute(ctx, *articleURL); err != nil {
		// Map domain errors to appropriate HTTP status codes
		if errors.Is(err, domain.ErrFeedNotFound) {
			h.logger.InfoContext(ctx, "feed not found for mark as read",
				"article_url", req.Msg.ArticleUrl,
				"error", err)
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("feed not found: %s", req.Msg.ArticleUrl))
		}

		// All other errors are internal server errors
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "MarkAsRead")
	}

	h.logger.InfoContext(ctx, "feed marked as read", "article_url", req.Msg.ArticleUrl)

	return connect.NewResponse(&feedsv2.MarkAsReadResponse{
		Message: "Feed read status updated",
	}), nil
}

// =============================================================================
// Subscription RPCs
// =============================================================================

// ListSubscriptions returns all feed sources with subscription status for the current user.
func (h *Handler) ListSubscriptions(
	ctx context.Context,
	req *connect.Request[feedsv2.ListSubscriptionsRequest],
) (*connect.Response[feedsv2.ListSubscriptionsResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	sources, err := h.container.ListSubscriptionsUsecase.Execute(ctx, userCtx.UserID)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "ListSubscriptions")
	}

	protoSources := make([]*feedsv2.FeedSource, 0, len(sources))
	for _, s := range sources {
		protoSources = append(protoSources, &feedsv2.FeedSource{
			Id:           s.ID,
			Url:          s.URL,
			Title:        s.Title,
			IsSubscribed: s.IsSubscribed,
			CreatedAt:    s.CreatedAt.Format(time.RFC3339),
		})
	}

	return connect.NewResponse(&feedsv2.ListSubscriptionsResponse{
		Sources: protoSources,
	}), nil
}

// Subscribe subscribes the current user to a feed source.
func (h *Handler) Subscribe(
	ctx context.Context,
	req *connect.Request[feedsv2.SubscribeRequest],
) (*connect.Response[feedsv2.SubscribeResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.FeedLinkId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_link_id is required"))
	}

	feedLinkID, err := uuid.Parse(req.Msg.FeedLinkId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid feed_link_id: %w", err))
	}

	if err := h.container.SubscribeUsecase.Execute(ctx, userCtx.UserID, feedLinkID); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "Subscribe")
	}

	return connect.NewResponse(&feedsv2.SubscribeResponse{
		Message: "Subscribed successfully",
	}), nil
}

// Unsubscribe unsubscribes the current user from a feed source.
func (h *Handler) Unsubscribe(
	ctx context.Context,
	req *connect.Request[feedsv2.UnsubscribeRequest],
) (*connect.Response[feedsv2.UnsubscribeResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.FeedLinkId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_link_id is required"))
	}

	feedLinkID, err := uuid.Parse(req.Msg.FeedLinkId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid feed_link_id: %w", err))
	}

	if err := h.container.UnsubscribeUsecase.Execute(ctx, userCtx.UserID, feedLinkID); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "Unsubscribe")
	}

	return connect.NewResponse(&feedsv2.UnsubscribeResponse{
		Message: "Unsubscribed successfully",
	}), nil
}
