// Package mqhub_connect provides Connect-RPC client for mq-hub service.
package mqhub_connect

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_Disabled(t *testing.T) {
	client := NewClient("http://localhost:9500", false)

	assert.NotNil(t, client)
	assert.False(t, client.IsEnabled())
}

func TestNewClient_Enabled(t *testing.T) {
	client := NewClient("http://localhost:9500", true)

	assert.NotNil(t, client)
	assert.True(t, client.IsEnabled())
}

func TestPublishArticleSummarized_Disabled(t *testing.T) {
	client := NewClient("http://localhost:9500", false)

	payload := ArticleSummarizedPayload{
		ArticleID: "test-article-id",
		UserID:    "test-user-id",
		Summary:   "This is a test summary.",
	}

	messageID, err := client.PublishArticleSummarized(context.Background(), payload)

	assert.NoError(t, err)
	assert.Empty(t, messageID, "Message ID should be empty when client is disabled")
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
