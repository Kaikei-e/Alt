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
