package mqhub_connect

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_Disabled(t *testing.T) {
	client, err := NewClient("http://localhost:9500", "", false)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.False(t, client.IsEnabled())
}

func TestNewClient_Enabled(t *testing.T) {
	f, err := os.CreateTemp("", "token")
	require.NoError(t, err)
	_, _ = f.WriteString("valid-token")
	_ = f.Close()
	defer os.Remove(f.Name())

	client, err := NewClient("http://localhost:9500", f.Name(), true)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.True(t, client.IsEnabled())
}

func TestNewClient_EmptyToken(t *testing.T) {
	f, err := os.CreateTemp("", "token")
	require.NoError(t, err)
	_, _ = f.WriteString("   \n")
	_ = f.Close()
	defer os.Remove(f.Name())

	client, err := NewClient("http://localhost:9500", f.Name(), true)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "token is empty")
}

func TestPublishArticleSummarized_Disabled(t *testing.T) {
	client, _ := NewClient("http://localhost:9500", "", false)
	payload := ArticleSummarizedPayload{}
	messageID, err := client.PublishArticleSummarized(context.Background(), payload)
	assert.NoError(t, err)
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
