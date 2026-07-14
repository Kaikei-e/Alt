package feeds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	feedsv2 "alt/gen/proto/alt/feeds/v2"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	"alt/utils/html_parser"
	"alt/utils/security"
	"alt/utils/url_validator"
)

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

	// Check cache for existing summary (skip when force refresh)
	forceRefresh := req.Msg.ForceRefresh != nil && *req.Msg.ForceRefresh
	if !forceRefresh {
		existingSummary, err := h.deps.AltDBRepository.FetchArticleSummaryByArticleID(ctx, resolvedArticleID)
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

	// Connect to pre-processor in a goroutine while sending heartbeats.
	// The pre-processor may block for minutes waiting for news-creator's
	// semaphore slot (batch jobs on remote Ollama hold local slots).
	type ppStreamResult struct {
		stream io.ReadCloser
		err    error
	}
	ppCh := make(chan ppStreamResult, 1)
	go func() {
		s, e := h.streamPreProcessorSummarize(ctx, resolvedContent, resolvedArticleID, resolvedTitle)
		ppCh <- ppStreamResult{stream: s, err: e}
	}()

	// Send heartbeats every 15s while waiting for pre-processor connection.
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
				return errorhandler.HandleInternalError(ctx, h.logger, result.err, "StreamSummarize.StartStream")
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

	// Stream chunks to client and capture full summary.
	// streamAndCaptureWithHeartbeat sends heartbeats while waiting for first LLM token.
	fullSummary, err := h.streamAndCaptureWithHeartbeat(ctx, stream, preProcessorStream, resolvedArticleID)
	if err != nil {
		return errorhandler.HandleInternalError(ctx, h.logger, err, "StreamSummarize.Streaming")
	}

	// Save summary to database
	if fullSummary != "" && resolvedArticleID != "" {
		if err := h.deps.AltDBRepository.SaveArticleSummary(ctx, resolvedArticleID, userCtx.UserID.String(), resolvedTitle, fullSummary); err != nil {
			h.logger.ErrorContext(ctx, "failed to save summary", "error", err, "article_id", resolvedArticleID)
			// Don't return error, streaming was successful
		} else {
			h.logger.InfoContext(ctx, "summary saved", "article_id", resolvedArticleID, "summary_length", len(fullSummary))

			// Also create summary version + knowledge event for Knowledge Home
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
		article, err := h.deps.AltDBRepository.FetchArticleByID(ctx, articleID)
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
	existingArticle, err := h.deps.AltDBRepository.FetchArticleByURL(ctx, feedURL)
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
		newArticleID, err := h.deps.AltDBRepository.SaveArticle(ctx, feedURL, title, content)
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
	newArticleID, err := h.deps.AltDBRepository.SaveArticle(ctx, feedURL, title, fetchedContent)
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

	// SSRF protection: basic validation
	if err := url_validator.IsAllowedURL(parsedURL); err != nil {
		return "", "", fmt.Errorf("URL not allowed: %w", err)
	}

	// SSRF protection: comprehensive validation with DNS rebinding prevention
	ssrfValidator := security.NewSSRFValidator()
	if err := ssrfValidator.ValidateURL(ctx, parsedURL); err != nil {
		return "", "", fmt.Errorf("ssrf validation failed: %w", err)
	}

	// Create secure HTTP client with connection-time IP validation
	secureClient := ssrfValidator.CreateSecureHTTPClient(10 * time.Second)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AltBot/1.0; +http://alt.com/bot)")

	// SSRF protection: URL validated by url_validator.IsAllowedURL() and SSRFValidator.ValidateURL().
	// secureClient created via SSRFValidator.CreateSecureHTTPClient() validates IPs at connection time.
	resp, err := secureClient.Do(req)
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

// streamPreProcessorSummarize calls the pre-processor streaming API via Connect-RPC.
// Uses an independent context with client-disconnect propagation to prevent zombie requests.
func (h *Handler) streamPreProcessorSummarize(ctx context.Context, content, articleID, title string) (io.ReadCloser, error) {
	if articleID == "" {
		return nil, fmt.Errorf("article_id is required")
	}

	// Create an independent context for the streaming request.
	// This prevents client disconnection (e.g., butterfly-facade timeout) from cancelling
	// the pre-processor stream mid-generation. Use 10-minute timeout for long articles.
	streamCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	// Monitor client context in a separate goroutine.
	// When client disconnects, cancel the pre-processor request to free GPU resources.
	go func() {
		select {
		case <-ctx.Done():
			h.logger.InfoContext(ctx, "client disconnected, cancelling pre-processor stream",
				"article_id", articleID,
				"reason", ctx.Err())
			cancel()
		case <-streamCtx.Done():
			// Stream completed normally or timed out
		}
	}()

	stream, err := h.deps.PreProcessorClient.StreamSummarize(streamCtx, content, articleID, title)
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("client disconnected during stream setup: %w", ctx.Err())
		}
		return nil, err
	}

	h.logger.InfoContext(ctx, "pre-processor Connect-RPC stream obtained", "article_id", articleID)

	return &streamReaderWithCancel{
		ReadCloser: stream,
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

// streamAndCaptureWithHeartbeat wraps streamAndCapture with heartbeat support.
// It sends empty chunks every 30s while waiting for the first real data from
// the pre-processor, preventing Cloudflare 524 timeout (100s idle limit).
// Once real data starts flowing, heartbeats stop and normal streaming takes over.
func (h *Handler) streamAndCaptureWithHeartbeat(
	ctx context.Context,
	stream *connect.ServerStream[feedsv2.StreamSummarizeResponse],
	preProcessorStream io.Reader,
	articleID string,
) (string, error) {
	// Phase 1: Wait for first data with heartbeats.
	// Read in a goroutine; send heartbeats from this goroutine while waiting.
	type readResult struct {
		buf []byte
		n   int
		err error
	}
	firstRead := make(chan readResult, 1)
	initialBuf := make([]byte, 256)
	go func() {
		n, err := preProcessorStream.Read(initialBuf)
		firstRead <- readResult{buf: initialBuf[:n], n: n, err: err}
	}()

	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	var first readResult
	waiting := true
	for waiting {
		select {
		case first = <-firstRead:
			waiting = false
		case <-heartbeatTicker.C:
			if sendErr := stream.Send(&feedsv2.StreamSummarizeResponse{
				Chunk: "", IsFinal: false, ArticleId: articleID,
			}); sendErr != nil {
				return "", sendErr
			}
			h.logger.DebugContext(ctx, "sent heartbeat while waiting for first chunk", "article_id", articleID)
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	heartbeatTicker.Stop()

	// Handle first read result
	if first.err != nil && first.err != io.EOF && first.n == 0 {
		return "", first.err
	}

	// Phase 2: Prepend initial data and delegate to normal streamAndCapture.
	// Wrap preProcessorStream with the already-read bytes prepended.
	var combinedReader io.Reader
	if first.n > 0 {
		combinedReader = io.MultiReader(
			strings.NewReader(string(first.buf[:first.n])),
			preProcessorStream,
		)
	} else {
		combinedReader = preProcessorStream
	}

	// If first read was EOF, we still need to process the data
	if first.err == io.EOF {
		combinedReader = strings.NewReader(string(first.buf[:first.n]))
	}

	return h.streamAndCapture(ctx, stream, combinedReader, articleID)
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

// preProcessorErrorResponse represents the JSON error response from pre-processor.
type preProcessorErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// mapPreProcessorHTTPError maps pre-processor HTTP error responses to Connect-RPC errors.
// Returns nil if the error cannot be mapped (caller should fall back to generic error).
func mapPreProcessorHTTPError(statusCode int, body []byte) *connect.Error {
	var errResp preProcessorErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return nil
	}
	switch errResp.Error.Code {
	case "VALIDATION_ERROR":
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s", errResp.Error.Message))
	case "CONFLICT_ERROR":
		return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("%s", errResp.Error.Message))
	default:
		return nil
	}
}
