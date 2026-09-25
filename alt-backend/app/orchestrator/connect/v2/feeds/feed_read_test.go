package feeds

import (
	"context"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/orchestrator/usecase/reading_status"
	"alt/orchestrator/usecase/resolve_article_usecase"
	"alt/utils/logger"
)

type mockUpdateArticleStatusPortReturnsNotFound struct{}

func (m *mockUpdateArticleStatusPortReturnsNotFound) MarkArticleAsRead(ctx context.Context, articleURL url.URL, userID uuid.UUID) error {
	return domain.ErrFeedNotFound
}

type mockUpdateArticleStatusPortReturnsError struct{}

func (m *mockUpdateArticleStatusPortReturnsError) MarkArticleAsRead(ctx context.Context, articleURL url.URL, userID uuid.UUID) error {
	return assert.AnError
}

func TestMarkAsRead_RequiresAuth(t *testing.T) {
	handler := createTestHandler()
	ctx := context.Background()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "https://example.com/article",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestMarkAsRead_RequiresArticleURL(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestMarkAsRead_InvalidURL(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "://invalid-url",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestMarkAsRead_Success(t *testing.T) {
	handler := createTestHandler()
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "https://example.com/article",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Feed read status updated", resp.Msg.Message)
}

func TestMarkAsReadResponse_Construction(t *testing.T) {
	resp := &feedsv2.MarkAsReadResponse{
		Message: "Feed read status updated",
	}

	assert.Equal(t, "Feed read status updated", resp.Message)
}

func TestHandler_MarkAsRead_FeedNotFound_Returns404(t *testing.T) {
	logger.InitLogger()

	articlesReadingStatusUsecase := reading_status.NewArticlesReadingStatusUsecase(&mockUpdateArticleStatusPortReturnsNotFound{})

	deps := FeedHandlerDeps{
		ArticlesReadingStatus: articlesReadingStatusUsecase,
		ResolveArticle:        resolve_article_usecase.New(nil, nil),
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handler := NewHandler(deps, cfg, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "https://example.com/nonexistent",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code(), "Expected HTTP 404 Not Found")
	assert.Contains(t, connectErr.Message(), "feed not found", "Error message should mention 'feed not found'")
}

func TestHandler_MarkAsRead_DatabaseError_Returns500(t *testing.T) {
	logger.InitLogger()

	articlesReadingStatusUsecase := reading_status.NewArticlesReadingStatusUsecase(&mockUpdateArticleStatusPortReturnsError{})

	deps := FeedHandlerDeps{
		ArticlesReadingStatus: articlesReadingStatusUsecase,
		ResolveArticle:        resolve_article_usecase.New(nil, nil),
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			SSEInterval: 5 * time.Second,
		},
	}

	handler := NewHandler(deps, cfg, slog.Default())
	ctx := createAuthContext()

	req := connect.NewRequest(&feedsv2.MarkAsReadRequest{
		ArticleUrl: "https://example.com/article",
	})

	resp, err := handler.MarkAsRead(ctx, req)

	require.Error(t, err)
	require.Nil(t, resp)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInternal, connectErr.Code(), "Expected HTTP 500 Internal Server Error")
	assert.Contains(t, connectErr.Message(), "An unexpected error occurred", "Error message should be generic and user-friendly")
	assert.Contains(t, connectErr.Message(), "Error ID:", "Error message should contain Error ID for traceability")
}
