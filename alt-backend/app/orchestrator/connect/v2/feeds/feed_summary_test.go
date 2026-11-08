package feeds

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	feedsv2 "alt/gen/proto/alt/feeds/v2"
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

func TestStreamSummarize_UnownedArticleIDReturnsNotFoundEvenWithContent(t *testing.T) {
	store := &stubArticleStore{byID: nil}
	handler := newResolveArticleHandler(store)
	ctx := createAuthContext()

	articleID := "11111111-1111-4111-8111-111111111111"
	content := "injected content for another user's article"
	title := "Injected Title"

	errWithContent := handler.StreamSummarize(ctx, connect.NewRequest(&feedsv2.StreamSummarizeRequest{
		ArticleId: &articleID,
		Content:   &content,
		Title:     &title,
	}), nil)
	require.Error(t, errWithContent)
	var connectErrWithContent *connect.Error
	require.ErrorAs(t, errWithContent, &connectErrWithContent)
	assert.Equal(t, connect.CodeNotFound, connectErrWithContent.Code())

	errWithoutContent := handler.StreamSummarize(ctx, connect.NewRequest(&feedsv2.StreamSummarizeRequest{
		ArticleId: &articleID,
	}), nil)
	require.Error(t, errWithoutContent)
	var connectErrWithoutContent *connect.Error
	require.ErrorAs(t, errWithoutContent, &connectErrWithoutContent)
	assert.Equal(t, connect.CodeNotFound, connectErrWithoutContent.Code())
	assert.Equal(t, connectErrWithoutContent.Message(), connectErrWithContent.Message())
	assert.Empty(t, store.savedSummaryID)
}
