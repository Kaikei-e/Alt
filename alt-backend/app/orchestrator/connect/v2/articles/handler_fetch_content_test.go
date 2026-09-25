package articles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	articlesv2 "alt/gen/proto/alt/articles/v2"
)

// mockArticleUsecase implements fetch_article_usecase.ArticleUsecase for testing.
type mockArticleUsecase struct {
	content    string
	articleID  string
	ogImageURL string
	err        error
}

func (m *mockArticleUsecase) Execute(ctx context.Context, articleURL string) (*string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &m.content, nil
}

func (m *mockArticleUsecase) FetchCompliantArticle(ctx context.Context, articleURL *url.URL, userContext domain.UserContext) (string, string, string, error) {
	if m.err != nil {
		return "", "", "", m.err
	}
	return m.content, m.articleID, m.ogImageURL, nil
}

func (m *mockArticleUsecase) FetchCompliantArticleWithRefresh(ctx context.Context, articleURL *url.URL, userContext domain.UserContext, forceRefresh bool) (string, string, string, error) {
	if m.err != nil {
		return "", "", "", m.err
	}
	return m.content, m.articleID, m.ogImageURL, nil
}

func TestFetchArticleContent_UpstreamFetchError_ReturnsUnavailableAndLogsWarn(t *testing.T) {
	tests := []struct {
		name  string
		cause error
	}{
		{"publisher timed out", context.DeadlineExceeded},
		{"host slot wait ran out", errors.New("rate: Wait(n=1) would exceed context deadline")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const articleURL = "https://www3.nhk.or.jp/news/article"
			mockUsecase := &mockArticleUsecase{
				err: fmt.Errorf("fetch failed: %w", &domain.UpstreamFetchError{URL: articleURL, Cause: tt.cause}),
			}
			var logBuf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			handler := NewHandler(ArticleHandlerDeps{Article: mockUsecase}, &config.Config{}, logger)

			req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{Url: articleURL})
			_, err := handler.FetchArticleContent(createAuthContext(), req)

			require.Error(t, err)
			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
			assert.Contains(t, connectErr.Message(), "site")
			assert.NotContains(t, connectErr.Message(), "Error ID")

			logged := logBuf.String()
			assert.Contains(t, logged, `"level":"WARN"`)
			assert.NotContains(t, logged, `"level":"ERROR"`)
			assert.Contains(t, logged, articleURL)
			assert.Contains(t, logged, tt.cause.Error())
		})
	}
}

func TestFetchArticleContent_HostSlotBusy_ReturnsResourceExhaustedWithRetryAfter(t *testing.T) {
	mockUsecase := &mockArticleUsecase{
		err: fmt.Errorf("fetch failed: %w", &domain.RateLimitedError{
			Message:    "This site is busy with another request. Please try again shortly.",
			RetryAfter: 10 * time.Second,
		}),
	}
	handler := NewHandler(ArticleHandlerDeps{Article: mockUsecase}, &config.Config{}, slog.Default())

	req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{Url: "https://www3.nhk.or.jp/news/article"})
	_, err := handler.FetchArticleContent(createAuthContext(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeResourceExhausted, connectErr.Code())
	assert.Equal(t, "10", connectErr.Meta().Get("Retry-After"))
	assert.NotContains(t, connectErr.Message(), "did not respond")
}

func TestFetchArticleContent_UnclassifiedError_StaysInternal(t *testing.T) {
	mockUsecase := &mockArticleUsecase{
		err: errors.New(`fetch failed: rate limit wait failed for "https://zenn.dev/article": boom`),
	}
	handler := NewHandler(ArticleHandlerDeps{Article: mockUsecase}, &config.Config{}, slog.Default())

	req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{Url: "https://zenn.dev/article"})
	_, err := handler.FetchArticleContent(createAuthContext(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
}

func TestFetchArticleContent_ExternalHTTPError_404_ReturnsNotFound(t *testing.T) {
	mockUsecase := &mockArticleUsecase{
		err: fmt.Errorf("fetch failed: %w", &domain.ExternalHTTPError{StatusCode: 404, URL: "https://example.com/deleted"}),
	}
	deps := ArticleHandlerDeps{
		Article: mockUsecase,
	}
	handler := NewHandler(deps, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{
		Url: "https://example.com/deleted",
	})
	_, err := handler.FetchArticleContent(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

func TestFetchArticleContent_ExternalHTTPError_410_ReturnsNotFound(t *testing.T) {
	mockUsecase := &mockArticleUsecase{
		err: fmt.Errorf("fetch failed: %w", &domain.ExternalHTTPError{StatusCode: 410, URL: "https://example.com/gone"}),
	}
	deps := ArticleHandlerDeps{
		Article: mockUsecase,
	}
	handler := NewHandler(deps, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{
		Url: "https://example.com/gone",
	})
	_, err := handler.FetchArticleContent(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

func TestFetchArticleContent_ExternalHTTPError_403_ReturnsPermissionDenied(t *testing.T) {
	mockUsecase := &mockArticleUsecase{
		err: fmt.Errorf("fetch failed: %w", &domain.ExternalHTTPError{StatusCode: 403, URL: "https://example.com/forbidden"}),
	}
	deps := ArticleHandlerDeps{
		Article: mockUsecase,
	}
	handler := NewHandler(deps, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{
		Url: "https://example.com/forbidden",
	})
	_, err := handler.FetchArticleContent(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
}

func TestFetchArticleContent_ExternalHTTPError_429_ReturnsResourceExhausted(t *testing.T) {
	mockUsecase := &mockArticleUsecase{
		err: fmt.Errorf("fetch failed: %w", &domain.ExternalHTTPError{StatusCode: 429, URL: "https://example.com/ratelimited"}),
	}
	deps := ArticleHandlerDeps{
		Article: mockUsecase,
	}
	handler := NewHandler(deps, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{
		Url: "https://example.com/ratelimited",
	})
	_, err := handler.FetchArticleContent(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeResourceExhausted, connectErr.Code())
}

func TestFetchArticleContent_ExternalHTTPError_500_ReturnsUnavailable(t *testing.T) {
	mockUsecase := &mockArticleUsecase{
		err: fmt.Errorf("fetch failed: %w", &domain.ExternalHTTPError{StatusCode: 500, URL: "https://example.com/broken"}),
	}
	deps := ArticleHandlerDeps{
		Article: mockUsecase,
	}
	handler := NewHandler(deps, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{
		Url: "https://example.com/broken",
	})
	_, err := handler.FetchArticleContent(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
}

func TestFetchArticleContent_PublisherAttributableErrors_CarryHostFailureScope(t *testing.T) {
	const articleURL = "https://example.com/article"

	tests := []struct {
		name string
		err  error
	}{
		{"publisher never answered", &domain.UpstreamFetchError{URL: articleURL, Cause: context.DeadlineExceeded}},
		{"publisher returned 500", &domain.ExternalHTTPError{StatusCode: 500, URL: articleURL}},
		{"publisher returned 406", &domain.ExternalHTTPError{StatusCode: 406, URL: articleURL}},
		{"publisher removed the article", &domain.ExternalHTTPError{StatusCode: 404, URL: articleURL}},
		{"publisher denied access", &domain.ExternalHTTPError{StatusCode: 403, URL: articleURL}},
		{"publisher rate limited us", &domain.ExternalHTTPError{StatusCode: 429, URL: articleURL}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUsecase := &mockArticleUsecase{err: fmt.Errorf("fetch failed: %w", tt.err)}
			handler := NewHandler(ArticleHandlerDeps{Article: mockUsecase}, &config.Config{}, slog.Default())

			req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{Url: articleURL})
			_, err := handler.FetchArticleContent(createAuthContext(), req)

			require.Error(t, err)
			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Equal(t, FailureScopeHost, connectErr.Meta().Get(FailureScopeHeader))
		})
	}
}

func TestFetchArticleContent_OurOwnFailures_CarryNoFailureScope(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"unclassified fault", errors.New("boom")},
		{"our host slot was busy", &domain.RateLimitedError{Message: "busy", RetryAfter: 10 * time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUsecase := &mockArticleUsecase{err: fmt.Errorf("fetch failed: %w", tt.err)}
			handler := NewHandler(ArticleHandlerDeps{Article: mockUsecase}, &config.Config{}, slog.Default())

			req := connect.NewRequest(&articlesv2.FetchArticleContentRequest{Url: "https://example.com/article"})
			_, err := handler.FetchArticleContent(createAuthContext(), req)

			require.Error(t, err)
			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			assert.Empty(t, connectErr.Meta().Get(FailureScopeHeader))
		})
	}
}

func TestFailureScope_ReachesTheWireAsAResponseHeader(t *testing.T) {
	handler := connect.NewUnaryHandler(
		"/test.v1.Service/Method",
		func(context.Context, *connect.Request[articlesv2.FetchArticleContentRequest]) (*connect.Response[articlesv2.FetchArticleContentResponse], error) {
			return nil, withFailureScope(
				connect.NewError(connect.CodeUnavailable, errors.New("the source site did not respond")),
				FailureScopeHost,
			)
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/test.v1.Service/Method", handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := server.Client().Post(
		server.URL+"/test.v1.Service/Method",
		"application/json",
		strings.NewReader(`{"url":"https://example.com/article"}`),
	)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, FailureScopeHost, resp.Header.Get(FailureScopeHeader))
}
