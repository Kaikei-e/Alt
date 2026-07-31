//go:build contract

package contract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/matchers"
	message "github.com/pact-foundation/pact-go/v2/message/v4"
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
// No consumer contract requires content/tags any more -- consumers dereference
// article_id against alt-backend -- but the fields stay here because alt-backend
// still emits them, and the fixtures below must keep satisfying the pacts of
// consumer versions that are still deployed.
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

// The ArticleCreated interactions on alt:events:articles are NOT declared here.
// mq-hub writes that stream and search-indexer reads it, so the contract belongs
// to search-indexer as consumer: pacts/search-indexer-mq-hub.json, verified
// against mq-hub's real event builders by TestVerifySearchIndexerMqHubMessagePact
// in provider_test.go. A mq-hub-as-consumer pact for the same interactions
// inverted the direction and could only ever be "verified" by mq-hub asserting
// on its own output.

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
