package mqhub_connect

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mqhubv1 "pre-processor/gen/proto/services/mqhub/v1"
)

func TestNewClient_Disabled(t *testing.T) {
	client, err := NewClient("http://localhost:9500", "", false)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.False(t, client.IsEnabled())
}

func TestNewClient_Enabled(t *testing.T) {
	f, err := os.CreateTemp("", "token")
	require.NoError(t, err)
	_, _ = f.WriteString("valid-token")
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	client, err := NewClient("http://localhost:9500", f.Name(), true)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.True(t, client.IsEnabled())
}

func TestNewClient_EmptyToken(t *testing.T) {
	f, err := os.CreateTemp("", "token")
	require.NoError(t, err)
	_, _ = f.WriteString("   \n")
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	client, err := NewClient("http://localhost:9500", f.Name(), true)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "token is empty")
}

func TestPublishArticleSummarized_Disabled(t *testing.T) {
	client, _ := NewClient("http://localhost:9500", "", false)
	payload := ArticleSummarizedPayload{}
	messageID, err := client.PublishArticleSummarized(context.Background(), payload)
	require.NoError(t, err)
	assert.Empty(t, messageID)
}

func TestArticleSummarizedPayload_MarshalJSON(t *testing.T) {
	payload := ArticleSummarizedPayload{
		ArticleID: "article-123",
		UserID:    "user-456",
		Summary:   "Test summary content",
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "article-123", decoded["article_id"])
	assert.Equal(t, "user-456", decoded["user_id"])
	assert.Equal(t, "Test summary content", decoded["summary"])
}

func TestClient_StreamKeysAndEventTypes(t *testing.T) {
	// Verify constants are defined correctly
	assert.Equal(t, "alt:events:summaries", StreamKeySummaries)
	assert.Equal(t, "ArticleSummarized", EventTypeArticleSummarized)
}

func TestPublishArticleSummarized_NilResponseFailsFast(t *testing.T) {
	nilMsgInterceptor := connect.UnaryInterceptorFunc(func(_ connect.UnaryFunc) connect.UnaryFunc {
		return func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
			var nilMsg *mqhubv1.PublishResponse
			return connect.NewResponse(nilMsg), nil
		}
	})

	client := newClientWithOptions("http://127.0.0.1:1", http.DefaultClient, connect.WithInterceptors(nilMsgInterceptor))
	_, err := client.PublishArticleSummarized(context.Background(), ArticleSummarizedPayload{
		ArticleID: "article-123",
		UserID:    "user-456",
		Summary:   "summary",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil response message")
}
