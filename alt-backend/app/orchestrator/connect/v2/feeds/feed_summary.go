package feeds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	feedsv2 "alt/gen/proto/alt/feeds/v2"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
)

// StreamSummarize streams article summarization in real-time.
func (h *Handler) StreamSummarize(
	ctx context.Context,
	req *connect.Request[feedsv2.StreamSummarizeRequest],
	stream *connect.ServerStream[feedsv2.StreamSummarizeResponse],
) error {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

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

	content := ""
	if req.Msg.Content != nil {
		content = *req.Msg.Content
	}
	title := ""
	if req.Msg.Title != nil {
		title = *req.Msg.Title
	}

	resolvedArticleID, resolvedTitle, resolvedContent, err := h.resolveArticle(ctx, feedURL, articleID, content, title)
	if err != nil {
		return errorhandler.HandleUpstreamError(ctx, h.logger, err, "StreamSummarize.ResolveArticle")
	}

	if resolvedContent == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("content cannot be empty for summarization"))
	}

	forceRefresh := req.Msg.ForceRefresh != nil && *req.Msg.ForceRefresh
	if !forceRefresh {
		existingSummary, err := h.deps.SummaryStore.FetchArticleSummaryByArticleID(ctx, resolvedArticleID)
		if err == nil && existingSummary != nil && existingSummary.Summary != "" {
			h.logger.InfoContext(ctx, "returning cached summary", "article_id", resolvedArticleID)
			return stream.Send(&feedsv2.StreamSummarizeResponse{
				Chunk:       "",
				IsFinal:     true,
				ArticleId:   resolvedArticleID,
				IsCached:    true,
				FullSummary: &existingSummary.Summary,
			})
		}
	} else {
		h.logger.InfoContext(ctx, "force refresh: skipping summary cache", "article_id", resolvedArticleID)
	}

	h.logger.InfoContext(ctx, "starting stream summarization",
		"article_id", resolvedArticleID,
		"content_length", len(resolvedContent))

	// Send initial heartbeat immediately to start the HTTP response.
	// This resets Cloudflare's 100s idle timer before the potentially slow
	// pre-processor connection is established (semaphore wait can take minutes).
	if sendErr := stream.Send(&feedsv2.StreamSummarizeResponse{
		Chunk: "", IsFinal: false, ArticleId: resolvedArticleID,
	}); sendErr != nil {
		return sendErr
	}

	type ppStreamResult struct {
		stream io.ReadCloser
		err    error
	}
	ppCh := make(chan ppStreamResult, 1)
	go func() {
		s, e := h.streamPreProcessorSummarize(ctx, resolvedContent, resolvedArticleID, resolvedTitle)
		ppCh <- ppStreamResult{stream: s, err: e}
	}()

	heartbeatTicker := time.NewTicker(15 * time.Second)
	var preProcessorStream io.ReadCloser
waitLoop:
	for {
		select {
		case result := <-ppCh:
			if result.err != nil {
				heartbeatTicker.Stop()
				var connectErr *connect.Error
				if errors.As(result.err, &connectErr) {
					h.logger.InfoContext(ctx, "pre-processor returned client error",
						"article_id", resolvedArticleID,
						"code", connectErr.Code(),
						"message", connectErr.Message())
					return connectErr
				}
				return errorhandler.HandleUpstreamError(ctx, h.logger, result.err, "StreamSummarize.StartStream")
			}
			preProcessorStream = result.stream
			break waitLoop
		case <-heartbeatTicker.C:
			if sendErr := stream.Send(&feedsv2.StreamSummarizeResponse{
				Chunk: "", IsFinal: false, ArticleId: resolvedArticleID,
			}); sendErr != nil {
				heartbeatTicker.Stop()
				return sendErr
			}
			h.logger.DebugContext(ctx, "sent heartbeat while waiting for pre-processor", "article_id", resolvedArticleID)
		case <-ctx.Done():
			heartbeatTicker.Stop()
			return ctx.Err()
		}
	}
	heartbeatTicker.Stop()

	defer func() {
		if closeErr := preProcessorStream.Close(); closeErr != nil {
			h.logger.DebugContext(ctx, "failed to close pre-processor stream", "error", closeErr)
		}
	}()

	fullSummary, err := h.streamAndCaptureWithHeartbeat(ctx, stream, preProcessorStream, resolvedArticleID)
	if err != nil {
		return errorhandler.HandleUpstreamError(ctx, h.logger, err, "StreamSummarize.Streaming")
	}

	if fullSummary != "" && resolvedArticleID != "" {
		if err := h.deps.ArticleStore.SaveArticleSummary(ctx, resolvedArticleID, userCtx.UserID.String(), resolvedTitle, fullSummary); err != nil {
			h.logger.ErrorContext(ctx, "failed to save summary", "error", err, "article_id", resolvedArticleID)
		} else {
			h.logger.InfoContext(ctx, "summary saved", "article_id", resolvedArticleID, "summary_length", len(fullSummary))

			if h.deps.CreateSummaryVersion != nil {
				articleUUID, parseErr := uuid.Parse(resolvedArticleID)
				if parseErr == nil {
					sv := domain.SummaryVersion{
						ArticleID:   articleUUID,
						UserID:      userCtx.UserID,
						SummaryText: fullSummary,
						Model:       "stream-summarize",
					}
					if svErr := h.deps.CreateSummaryVersion.Execute(ctx, sv); svErr != nil {
						h.logger.ErrorContext(ctx, "failed to create summary version", "error", svErr, "article_id", resolvedArticleID)
					}
				}
			}
		}
	}

	return stream.Send(&feedsv2.StreamSummarizeResponse{
		Chunk:       "",
		IsFinal:     true,
		ArticleId:   resolvedArticleID,
		IsCached:    false,
		FullSummary: &fullSummary,
	})
}
