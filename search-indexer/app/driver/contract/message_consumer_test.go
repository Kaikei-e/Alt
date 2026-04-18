//go:build contract

package contract

import (
	"encoding/json"
	"testing"
	"time"

	message "github.com/pact-foundation/pact-go/v2/message/v4"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RedisStreamEvent represents the wire format of an event on Redis Streams
// as serialized by mq-hub's RedisDriver.eventToValues().
type RedisStreamEvent struct {
	EventID   string            `json:"event_id"`
	EventType string            `json:"event_type"`
	Source    string            `json:"source"`
	CreatedAt string            `json:"created_at"`
	Payload   json.RawMessage   `json:"payload"`
	Metadata  map[string]string `json:"metadata"`
}

// ArticleCreatedPayload mirrors consumer.ArticleCreatedPayload for contract testing.
type ArticleCreatedPayload struct {
	ArticleID   string   `json:"article_id"`
	UserID      string   `json:"user_id"`
	FeedID      string   `json:"feed_id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Content     string   `json:"content,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	PublishedAt string   `json:"published_at"`
}

func TestConsumeArticleCreatedEvent(t *testing.T) {
	p, err := message.NewAsynchronousPact(message.Config{
		Consumer: "search-indexer",
		Provider: "mq-hub",
		PactDir:  pactDir,
	})
	require.NoError(t, err)

	err = p.AddAsynchronousMessage().
		Given("the articles stream exists").
		ExpectsToReceive("an ArticleCreated event on alt:events:articles").
		WithJSONContent(matchers.MapMatcher{
			"event_id":   matchers.Like("evt-uuid-001"),
			"event_type": matchers.String("ArticleCreated"),
			"source":     matchers.Like("alt-backend"),
			"created_at": matchers.Like("2026-03-26T00:00:00.000Z"),
			"payload": matchers.Like(matchers.MapMatcher{
				"article_id":   matchers.Like("art-001"),
				"user_id":      matchers.Like("user-001"),
				"feed_id":      matchers.Like("feed-001"),
				"title":        matchers.Like("Test Article"),
				"url":          matchers.Like("https://example.com/article"),
				"published_at": matchers.Like("2026-03-26T00:00:00Z"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"trace_id": matchers.Like("trace-001"),
			}),
		}).
		AsType(&RedisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			// Simulate search-indexer's consumer.parseEvent -> event_handler.HandleEvent path.
			// Use Contents ([]byte) for unmarshalling, not Body (interface{}).
			var wireEvent RedisStreamEvent
			err := json.Unmarshal(contents.Contents, &wireEvent)
			require.NoError(t, err)

			// Verify envelope fields that search-indexer reads
			assert.NotEmpty(t, wireEvent.EventID, "event_id is required")
			assert.Equal(t, "ArticleCreated", wireEvent.EventType)
			assert.NotEmpty(t, wireEvent.Source, "source is required")

			// Verify created_at can be parsed (search-indexer uses time.Parse(time.RFC3339, ...))
			_, err = time.Parse(time.RFC3339, wireEvent.CreatedAt)
			if err != nil {
				// Also try the millisecond format that mq-hub uses
				_, err = time.Parse("2006-01-02T15:04:05.000Z07:00", wireEvent.CreatedAt)
			}
			assert.NoError(t, err, "created_at must be parseable as RFC3339")

			// Verify payload can be deserialized to ArticleCreatedPayload
			var payload ArticleCreatedPayload
			err = json.Unmarshal(wireEvent.Payload, &payload)
			require.NoError(t, err)
			assert.NotEmpty(t, payload.ArticleID, "article_id is required in payload")
			assert.NotEmpty(t, payload.Title, "title is required in payload")

			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

func TestConsumeArticleCreatedFatEvent(t *testing.T) {
	p, err := message.NewAsynchronousPact(message.Config{
		Consumer: "search-indexer",
		Provider: "mq-hub",
		PactDir:  pactDir,
	})
	require.NoError(t, err)

	err = p.AddAsynchronousMessage().
		Given("the articles stream exists").
		ExpectsToReceive("an ArticleCreated fat event with content on alt:events:articles").
		WithJSONContent(matchers.MapMatcher{
			"event_id":   matchers.Like("evt-uuid-fat-001"),
			"event_type": matchers.String("ArticleCreated"),
			"source":     matchers.Like("alt-backend"),
			"created_at": matchers.Like("2026-03-26T00:00:00.000Z"),
			"payload": matchers.Like(matchers.MapMatcher{
				"article_id":   matchers.Like("art-001"),
				"user_id":      matchers.Like("user-001"),
				"feed_id":      matchers.Like("feed-001"),
				"title":        matchers.Like("Test Article"),
				"url":          matchers.Like("https://example.com/article"),
				"content":      matchers.Like("Full article content for direct indexing."),
				"tags":         matchers.EachLike(matchers.Like("technology"), 1),
				"published_at": matchers.Like("2026-03-26T00:00:00Z"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"trace_id": matchers.Like("trace-001"),
			}),
		}).
		AsType(&RedisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			var wireEvent RedisStreamEvent
			err := json.Unmarshal(contents.Contents, &wireEvent)
			require.NoError(t, err)

			assert.Equal(t, "ArticleCreated", wireEvent.EventType)

			// Verify fat event payload includes content and tags
			var payload ArticleCreatedPayload
			err = json.Unmarshal(wireEvent.Payload, &payload)
			require.NoError(t, err)
			assert.NotEmpty(t, payload.ArticleID)
			assert.NotEmpty(t, payload.Content, "fat event must include content for direct indexing")
			assert.NotEmpty(t, payload.Tags, "fat event must include tags")
			assert.NotEmpty(t, payload.UserID, "user_id is needed for search document")

			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

// TestConsumeArticleUpdatedFatEvent locks in that the ArticleUpdated event
// must be carried by the same stream and shaped identically to ArticleCreated.
// Without this pact, the provider (alt-backend via mq-hub) was free to add
// the event type while the consumer silently dropped it — exactly the
// regression observed on 2026-04-18 where the search index went stale on
// every article edit.
func TestConsumeArticleUpdatedFatEvent(t *testing.T) {
	p, err := message.NewAsynchronousPact(message.Config{
		Consumer: "search-indexer",
		Provider: "mq-hub",
		PactDir:  pactDir,
	})
	require.NoError(t, err)

	err = p.AddAsynchronousMessage().
		Given("the articles stream exists and an article was updated").
		ExpectsToReceive("an ArticleUpdated fat event on alt:events:articles").
		WithJSONContent(matchers.MapMatcher{
			"event_id":   matchers.Like("evt-uuid-upd-001"),
			"event_type": matchers.String("ArticleUpdated"),
			"source":     matchers.Like("alt-backend"),
			"created_at": matchers.Like("2026-03-26T00:00:00.000Z"),
			"payload": matchers.Like(matchers.MapMatcher{
				"article_id":   matchers.Like("art-001"),
				"user_id":      matchers.Like("user-001"),
				"feed_id":      matchers.Like("feed-001"),
				"title":        matchers.Like("Updated Article"),
				"url":          matchers.Like("https://example.com/article"),
				"content":      matchers.Like("Updated article content."),
				"tags":         matchers.EachLike(matchers.Like("technology"), 1),
				"published_at": matchers.Like("2026-03-26T00:00:00Z"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"trace_id": matchers.Like("trace-001"),
			}),
		}).
		AsType(&RedisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			var wireEvent RedisStreamEvent
			err := json.Unmarshal(contents.Contents, &wireEvent)
			require.NoError(t, err)

			assert.Equal(t, "ArticleUpdated", wireEvent.EventType)

			// ArticleUpdated shares the fat-event payload with ArticleCreated
			// so search-indexer can upsert via the same code path.
			var payload ArticleCreatedPayload
			err = json.Unmarshal(wireEvent.Payload, &payload)
			require.NoError(t, err)
			assert.NotEmpty(t, payload.ArticleID)
			assert.NotEmpty(t, payload.Content, "fat event must include fresh content for re-indexing")
			assert.NotEmpty(t, payload.UserID, "user_id is needed for search document")

			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

func TestConsumeIndexArticleEvent(t *testing.T) {
	p, err := message.NewAsynchronousPact(message.Config{
		Consumer: "search-indexer",
		Provider: "mq-hub",
		PactDir:  pactDir,
	})
	require.NoError(t, err)

	err = p.AddAsynchronousMessage().
		Given("the index stream exists").
		ExpectsToReceive("an IndexArticle event on alt:events:index").
		WithJSONContent(matchers.MapMatcher{
			"event_id":   matchers.Like("evt-uuid-idx-001"),
			"event_type": matchers.String("IndexArticle"),
			"source":     matchers.Like("alt-backend"),
			"created_at": matchers.Like("2026-03-26T00:00:00.000Z"),
			"payload": matchers.Like(matchers.MapMatcher{
				"article_id": matchers.Like("art-003"),
				"user_id":    matchers.Like("user-001"),
				"feed_id":    matchers.Like("feed-001"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"trace_id": matchers.Like("trace-003"),
			}),
		}).
		AsType(&RedisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			var wireEvent RedisStreamEvent
			err := json.Unmarshal(contents.Contents, &wireEvent)
			require.NoError(t, err)

			assert.Equal(t, "IndexArticle", wireEvent.EventType)

			// IndexArticlePayload requires article_id
			type IndexArticlePayload struct {
				ArticleID string `json:"article_id"`
				UserID    string `json:"user_id"`
				FeedID    string `json:"feed_id"`
			}
			var payload IndexArticlePayload
			err = json.Unmarshal(wireEvent.Payload, &payload)
			require.NoError(t, err)
			assert.NotEmpty(t, payload.ArticleID, "article_id is required for index lookup")

			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}
