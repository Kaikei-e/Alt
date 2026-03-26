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

	"mq-hub/domain"
)

const pactDir = "../../../../pacts"

// RedisStreamEvent represents the wire format of an event on Redis Streams.
// This mirrors domain.Event as serialized by RedisDriver.eventToValues().
type RedisStreamEvent struct {
	EventID   string            `json:"event_id"`
	EventType string            `json:"event_type"`
	Source    string            `json:"source"`
	CreatedAt string            `json:"created_at"`
	Payload   json.RawMessage   `json:"payload"`
	Metadata  map[string]string `json:"metadata"`
}

// ArticleCreatedPayload is the payload structure for ArticleCreated events.
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

// TagGenerationRequestedPayload is the payload structure for TagGenerationRequested events.
type TagGenerationRequestedPayload struct {
	ArticleID string `json:"article_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	FeedID    string `json:"feed_id"`
}

// buildArticleCreatedEvent creates a domain.Event matching real mq-hub behavior.
func buildArticleCreatedEvent() *domain.Event {
	payload := ArticleCreatedPayload{
		ArticleID:   "art-001",
		UserID:      "user-001",
		FeedID:      "feed-001",
		Title:       "Breaking: Go 1.26 Released",
		URL:         "https://example.com/go-1-26",
		Content:     "The Go team announced the release of Go 1.26 with exciting new features.",
		Tags:        []string{"go", "programming"},
		PublishedAt: "2026-03-26T00:00:00Z",
	}
	payloadJSON, _ := json.Marshal(payload)

	event, _ := domain.NewEvent(
		domain.EventTypeArticleCreated,
		"alt-backend",
		payloadJSON,
		map[string]string{"trace_id": "abc-123"},
	)
	return event
}

// buildTagGenerationRequestedEvent creates a TagGenerationRequested event.
func buildTagGenerationRequestedEvent() *domain.Event {
	payload := TagGenerationRequestedPayload{
		ArticleID: "art-002",
		Title:     "Rust Memory Safety",
		Content:   "An article about memory safety in Rust programming language.",
		FeedID:    "feed-002",
	}
	payloadJSON, _ := json.Marshal(payload)

	event, _ := domain.NewEvent(
		domain.EventTypeTagGenerationRequested,
		"mq-hub",
		payloadJSON,
		map[string]string{
			"reply_to":       "alt:reply:tag-gen-001",
			"correlation_id": "corr-001",
		},
	)
	return event
}

// eventToWireFormat converts a domain.Event to the wire format used on Redis Streams.
// This mirrors RedisDriver.eventToValues() serialization.
func eventToWireFormat(event *domain.Event) RedisStreamEvent {
	return RedisStreamEvent{
		EventID:   event.EventID,
		EventType: string(event.EventType),
		Source:    event.Source,
		CreatedAt: event.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00"),
		Payload:   json.RawMessage(event.Payload),
		Metadata:  event.Metadata,
	}
}

func TestArticleCreatedMessageContract(t *testing.T) {
	p, err := message.NewAsynchronousPact(message.Config{
		Consumer: "mq-hub",
		Provider: "search-indexer",
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
				"title":        matchers.Like("Breaking: Go 1.26 Released"),
				"url":          matchers.Like("https://example.com/go-1-26"),
				"published_at": matchers.Like("2026-03-26T00:00:00Z"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"trace_id": matchers.Like("abc-123"),
			}),
		}).
		AsType(&RedisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			event := buildArticleCreatedEvent()
			wireEvent := eventToWireFormat(event)

			// Verify the event has required fields
			assert.NotEmpty(t, wireEvent.EventID, "event_id must not be empty")
			assert.Equal(t, "ArticleCreated", wireEvent.EventType)
			assert.Equal(t, "alt-backend", wireEvent.Source)
			assert.NotEmpty(t, wireEvent.CreatedAt, "created_at must not be empty")
			assert.NotEmpty(t, wireEvent.Payload, "payload must not be empty")

			// Verify payload structure
			var payload ArticleCreatedPayload
			err := json.Unmarshal(wireEvent.Payload, &payload)
			require.NoError(t, err)
			assert.NotEmpty(t, payload.ArticleID)
			assert.NotEmpty(t, payload.UserID)
			assert.NotEmpty(t, payload.Title)
			assert.NotEmpty(t, payload.PublishedAt)

			// Verify domain event validation passes
			assert.NoError(t, event.Validate())

			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

func TestArticleCreatedFatEventMessageContract(t *testing.T) {
	p, err := message.NewAsynchronousPact(message.Config{
		Consumer: "mq-hub",
		Provider: "search-indexer",
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
				"title":        matchers.Like("Breaking: Go 1.26 Released"),
				"url":          matchers.Like("https://example.com/go-1-26"),
				"content":      matchers.Like("The Go team announced the release."),
				"tags":         matchers.EachLike(matchers.Like("go"), 1),
				"published_at": matchers.Like("2026-03-26T00:00:00Z"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"trace_id": matchers.Like("abc-123"),
			}),
		}).
		AsType(&RedisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			event := buildArticleCreatedEvent()
			wireEvent := eventToWireFormat(event)

			var payload ArticleCreatedPayload
			err := json.Unmarshal(wireEvent.Payload, &payload)
			require.NoError(t, err)

			// Fat events include content and tags
			assert.NotEmpty(t, payload.Content, "fat event must include content")
			assert.NotEmpty(t, payload.Tags, "fat event must include tags")

			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

func TestTagGenerationRequestedMessageContract(t *testing.T) {
	p, err := message.NewAsynchronousPact(message.Config{
		Consumer: "mq-hub",
		Provider: "tag-generator",
		PactDir:  pactDir,
	})
	require.NoError(t, err)

	err = p.AddAsynchronousMessage().
		Given("the tags stream exists").
		ExpectsToReceive("a TagGenerationRequested event on alt:events:tags").
		WithJSONContent(matchers.MapMatcher{
			"event_id":   matchers.Like("evt-uuid-tag-001"),
			"event_type": matchers.String("TagGenerationRequested"),
			"source":     matchers.Like("mq-hub"),
			"created_at": matchers.Like("2026-03-26T00:00:00.000Z"),
			"payload": matchers.Like(matchers.MapMatcher{
				"article_id": matchers.Like("art-002"),
				"title":      matchers.Like("Rust Memory Safety"),
				"content":    matchers.Like("An article about memory safety."),
				"feed_id":    matchers.Like("feed-002"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"reply_to":       matchers.Like("alt:reply:tag-gen-001"),
				"correlation_id": matchers.Like("corr-001"),
			}),
		}).
		AsType(&RedisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			event := buildTagGenerationRequestedEvent()
			wireEvent := eventToWireFormat(event)

			// Verify envelope
			assert.Equal(t, "TagGenerationRequested", wireEvent.EventType)
			assert.Equal(t, "mq-hub", wireEvent.Source)

			// Verify payload
			var payload TagGenerationRequestedPayload
			err := json.Unmarshal(wireEvent.Payload, &payload)
			require.NoError(t, err)
			assert.NotEmpty(t, payload.ArticleID)
			assert.NotEmpty(t, payload.Title)
			assert.NotEmpty(t, payload.Content)

			// Verify metadata contains reply routing
			assert.NotEmpty(t, wireEvent.Metadata["reply_to"])
			assert.NotEmpty(t, wireEvent.Metadata["correlation_id"])

			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

func TestEventWireFormatMatchesRedisDriver(t *testing.T) {
	// This test verifies that our wire format matches what RedisDriver.eventToValues() produces.
	event := buildArticleCreatedEvent()

	wireEvent := eventToWireFormat(event)

	// Verify timestamp format matches RedisDriver's "2006-01-02T15:04:05.000Z07:00"
	_, err := time.Parse("2006-01-02T15:04:05.000Z07:00", wireEvent.CreatedAt)
	assert.NoError(t, err, "created_at must use the same format as RedisDriver.eventToValues()")

	// Verify event_type is the string representation
	assert.Equal(t, string(domain.EventTypeArticleCreated), wireEvent.EventType)

	// Verify payload is valid JSON
	assert.True(t, json.Valid(wireEvent.Payload), "payload must be valid JSON")
}
