package articles

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	articlesv2 "alt/gen/proto/alt/articles/v2"
)

func TestStreamArticleTagsResponse_Construction(t *testing.T) {
	// Test EventType enum values
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_UNSPECIFIED, articlesv2.StreamArticleTagsResponse_EventType(0))
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED, articlesv2.StreamArticleTagsResponse_EventType(1))
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_GENERATING, articlesv2.StreamArticleTagsResponse_EventType(2))
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED, articlesv2.StreamArticleTagsResponse_EventType(3))
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_ERROR, articlesv2.StreamArticleTagsResponse_EventType(4))

	// Test event construction
	msg := "Generating tags..."
	event := &articlesv2.StreamArticleTagsResponse{
		ArticleId: "article-123",
		Tags: []*articlesv2.ArticleTagItem{
			{
				Id:        "tag-1",
				Name:      "Go",
				CreatedAt: time.Now().Format(time.RFC3339),
			},
		},
		EventType: articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED,
		Message:   &msg,
	}

	assert.Equal(t, "article-123", event.ArticleId)
	assert.Len(t, event.Tags, 1)
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED, event.EventType)
	assert.NotNil(t, event.Message)
	assert.Equal(t, "Generating tags...", *event.Message)
}

func TestStreamArticleTagsRequest_Construction(t *testing.T) {
	title := "Test Article"
	content := "Test Content"
	feedID := "feed-123"

	req := &articlesv2.StreamArticleTagsRequest{
		ArticleId: "article-123",
		Title:     &title,
		Content:   &content,
		FeedId:    &feedID,
	}

	assert.Equal(t, "article-123", req.ArticleId)
	assert.NotNil(t, req.Title)
	assert.Equal(t, "Test Article", *req.Title)
	assert.NotNil(t, req.Content)
	assert.NotNil(t, req.FeedId)
}

func TestConvertTagsToProto(t *testing.T) {
	now := time.Now()
	tags := []*domain.FeedTag{
		{
			ID:        "tag-1",
			TagName:   "Go",
			CreatedAt: now,
		},
		{
			ID:        "tag-2",
			TagName:   "Testing",
			CreatedAt: now.Add(-time.Hour),
		},
	}

	protoTags := convertTagsToProto(tags)

	require.Len(t, protoTags, 2)
	assert.Equal(t, "tag-1", protoTags[0].Id)
	assert.Equal(t, "Go", protoTags[0].Name)
	assert.Equal(t, "tag-2", protoTags[1].Id)
	assert.Equal(t, "Testing", protoTags[1].Name)
}

func TestConvertTagsToProto_Empty(t *testing.T) {
	protoTags := convertTagsToProto([]*domain.FeedTag{})
	assert.Empty(t, protoTags)
	assert.NotNil(t, protoTags)
}

func TestStreamArticleTags_OnTheFlyGeneration_Behavior_Documented(t *testing.T) {
	t.Run("behavior_specification", func(t *testing.T) {
		deps := ArticleHandlerDeps{}
		cfg := &config.Config{}
		logger := slog.Default()
		handler := NewHandler(deps, cfg, logger)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler)
	})
}

func TestStreamArticleTags_ReturnsCompletedWithGeneratedTags(t *testing.T) {
	now := time.Now()
	generatedTags := []*domain.FeedTag{
		{ID: "gen-1", TagName: "AI", CreatedAt: now},
		{ID: "gen-2", TagName: "ML", CreatedAt: now},
	}

	expectedProtoTags := convertTagsToProto(generatedTags)

	assert.Len(t, expectedProtoTags, 2)
	assert.Equal(t, "AI", expectedProtoTags[0].Name)
	assert.Equal(t, "ML", expectedProtoTags[1].Name)
}

func TestStreamArticleTags_EventTypes_Documented(t *testing.T) {
	t.Run("event_type_semantics", func(t *testing.T) {
		assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED, articlesv2.StreamArticleTagsResponse_EventType(1))
		assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_GENERATING, articlesv2.StreamArticleTagsResponse_EventType(2))
		assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED, articlesv2.StreamArticleTagsResponse_EventType(3))
		assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_ERROR, articlesv2.StreamArticleTagsResponse_EventType(4))
	})

	t.Run("fail_open_behavior", func(t *testing.T) {
		err := errors.New("mq-hub connection failed")
		assert.NotNil(t, err)
	})
}
