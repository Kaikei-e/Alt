package feeds

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"alt/orchestrator/driver/preprocessor_connect"
	"alt/orchestrator/usecase/resolve_article_usecase"
	"alt/utils/logger"
)

func TestStreamSummarize_RequiresAuth(t *testing.T) {
	handler := createTestHandler()
	ctx := context.Background()

	feedURL := "https://example.com/article"
	req := connect.NewRequest(&feedsv2.StreamSummarizeRequest{
		FeedUrl: &feedURL,
	})

	err := handler.StreamSummarize(ctx, req, nil)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestStreamSummarize_RequiresFeedURLOrArticleID(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.StreamSummarizeRequest{})

	err := handler.StreamSummarize(ctx, req, nil)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestStreamSummarizeResponse_Construction(t *testing.T) {
	articleID := "test-article-id"
	summary := "This is a test summary"

	cachedResp := &feedsv2.StreamSummarizeResponse{
		Chunk:       "",
		IsFinal:     true,
		ArticleId:   articleID,
		IsCached:    true,
		FullSummary: &summary,
	}

	assert.Equal(t, articleID, cachedResp.ArticleId)
	assert.True(t, cachedResp.IsCached)
	assert.True(t, cachedResp.IsFinal)
	assert.Equal(t, summary, *cachedResp.FullSummary)

	chunkText := "This is a chunk"
	chunkResp := &feedsv2.StreamSummarizeResponse{
		Chunk:     chunkText,
		IsFinal:   false,
		ArticleId: articleID,
		IsCached:  false,
	}

	assert.Equal(t, chunkText, chunkResp.Chunk)
	assert.False(t, chunkResp.IsFinal)
	assert.False(t, chunkResp.IsCached)
}

type trackingUpstreamBody struct {
	closed    atomic.Bool
	closeOnce sync.Once
	closedCh  chan struct{}
}

func newTrackingUpstreamBody() *trackingUpstreamBody {
	return &trackingUpstreamBody{
		closedCh: make(chan struct{}),
	}
}

func (b *trackingUpstreamBody) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (b *trackingUpstreamBody) Close() error {
	b.closed.Store(true)
	b.closeOnce.Do(func() {
		close(b.closedCh)
	})
	return nil
}

type fakeUpstreamHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (f *fakeUpstreamHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f.doFunc(req)
}

type failOnSecondSendConn struct {
	connect.StreamingHandlerConn
	sendCount atomic.Int32
	sendErr   error
}

func (c *failOnSecondSendConn) Send(msg any) error {
	if c.sendCount.Add(1) >= 2 {
		return c.sendErr
	}
	return c.StreamingHandlerConn.Send(msg)
}

type streamSummarizeTestInterceptor struct {
	userCtx *domain.UserContext
	sendErr error
}

func (i *streamSummarizeTestInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return next
}

func (i *streamSummarizeTestInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *streamSummarizeTestInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx = domain.SetUserContext(ctx, i.userCtx)
		if i.sendErr != nil {
			conn = &failOnSecondSendConn{
				StreamingHandlerConn: conn,
				sendErr:              i.sendErr,
			}
		}
		return next(ctx, conn)
	}
}

func TestStreamSummarize_EarlyReturnClosesUpstreamAndCancelsStreamCtx(t *testing.T) {
	logger.InitLogger()

	t.Run("heartbeat Send error before upstream delivers stream closes stream and cancels streamCtx", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			upstreamStarted := make(chan struct{})
			releaseUpstream := make(chan struct{})
			upstreamBody := newTrackingUpstreamBody()

			var capturedStreamCtx context.Context
			fakeClient := &fakeUpstreamHTTPClient{
				doFunc: func(req *http.Request) (*http.Response, error) {
					capturedStreamCtx = req.Context()
					close(upstreamStarted)
					<-releaseUpstream
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/connect+json"},
						},
						Body: upstreamBody,
					}, nil
				},
			}

			store := &stubArticleStore{
				byID: &domain.ArticleContent{
					ID:      "art-1",
					Title:   "Article Title",
					Content: "Article body content to summarize",
					URL:     "https://example.com/art-1",
				},
			}
			ppClient := preprocessor_connect.NewConnectPreProcessorClientWithHTTPClient("https://pre-processor:9443", fakeClient)
			handler := NewHandler(FeedHandlerDeps{
				ArticleStore:       store,
				ResolveArticle:     resolve_article_usecase.New(store, &stubExtractor{}),
				PreProcessorClient: ppClient,
			}, &config.Config{}, slog.Default())

			authCtx := createAuthContext()
			userCtx, err := domain.GetUserFromContext(authCtx)
			require.NoError(t, err)

			_, svcHandler := feedsv2connect.NewFeedServiceHandler(
				handler,
				connect.WithInterceptors(&streamSummarizeTestInterceptor{
					userCtx: userCtx,
					sendErr: errors.New("client disconnected on heartbeat"),
				}),
			)

			reqCtx, cancelReq := context.WithCancel(context.Background())
			defer cancelReq()

			payload := `{"articleId":"art-1","forceRefresh":true}`
			framed := append([]byte{0, 0, 0, 0, byte(len(payload))}, []byte(payload)...)
			httpReq := httptest.NewRequest(
				http.MethodPost,
				feedsv2connect.FeedServiceStreamSummarizeProcedure,
				strings.NewReader(string(framed)),
			).WithContext(reqCtx)
			httpReq.Header.Set("Content-Type", "application/connect+json")
			rec := httptest.NewRecorder()

			handlerDone := make(chan struct{})
			go func() {
				defer close(handlerDone)
				svcHandler.ServeHTTP(rec, httpReq)
			}()

			// Wait for the background goroutine to start the upstream request.
			<-upstreamStarted

			// Advance virtual time inside the bubble so the 15s heartbeat ticker fires
			// and StreamSummarize returns early due to the second Send failing.
			<-handlerDone

			// Now the fake upstream delivers its stream after the handler already returned.
			close(releaseUpstream)
			synctest.Wait()

			require.NotNil(t, capturedStreamCtx)
			assert.True(t, upstreamBody.closed.Load(), "expected upstream stream Close to be called when delivered after handler returned")
			assert.Error(t, capturedStreamCtx.Err(), "expected 10-minute streamCtx to be cancelled when handler returned early")
		})
	})
}
