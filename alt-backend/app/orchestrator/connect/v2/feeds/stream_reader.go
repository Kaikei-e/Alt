package feeds

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"connectrpc.com/connect"

	feedsv2 "alt/gen/proto/alt/feeds/v2"
)

// streamPreProcessorSummarize calls the pre-processor streaming API with disconnect cancellation.
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

			for {
				sseData := sseBuf.String()
				splitIdx := strings.Index(sseData, "\n\n")
				if splitIdx == -1 {
					break
				}

				eventStr := sseData[:splitIdx]
				sseBuf.Reset()
				sseBuf.WriteString(sseData[splitIdx+2:])

				dataContent := extractSSEData(eventStr)
				if dataContent != "" {
					summaryBuf.WriteString(dataContent)

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

// streamAndCaptureWithHeartbeat wraps streamAndCapture with heartbeat support before first token.
func (h *Handler) streamAndCaptureWithHeartbeat(
	ctx context.Context,
	stream *connect.ServerStream[feedsv2.StreamSummarizeResponse],
	preProcessorStream io.Reader,
	articleID string,
) (string, error) {
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

	if first.err != nil && first.err != io.EOF && first.n == 0 {
		return "", first.err
	}

	var combinedReader io.Reader
	if first.n > 0 {
		combinedReader = io.MultiReader(
			strings.NewReader(string(first.buf[:first.n])),
			preProcessorStream,
		)
	} else {
		combinedReader = preProcessorStream
	}

	if first.err == io.EOF {
		combinedReader = strings.NewReader(string(first.buf[:first.n]))
	}

	return h.streamAndCapture(ctx, stream, combinedReader, articleID)
}
