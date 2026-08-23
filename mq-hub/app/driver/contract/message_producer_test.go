//go:build contract

package contract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"

	"mq-hub/domain"
)

const pactDir = "../../../../pacts"

// publishedAtFixture is the article publish time used across the article-event
// fixtures. It is a time.Time (not a string) so ArticleCreatedPayload below can
// mirror alt-backend's real producer struct field for field; a UTC time with no
// sub-second component marshals to "2026-03-26T00:00:00Z", exactly what the real
// producer emits.
var publishedAtFixture = time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)

// producerTraceMetadata reproduces the Metadata map alt-backend's real mq-hub
// producer attaches to every article event (shared/driver/mqhub_connect.
// traceMetadata): trace_id and span_id derived from the live OTel span context.
// mq-hub cannot import module `alt`, so the mechanism is replicated here against
// the same go.opentelemetry.io/otel/trace API. Using a span-derived 32-/16-hex id
// instead of a hand-typed placeholder like "abc-123" keeps the verified fixture
// honest about the shape the producer actually puts on the wire (CDC-1): the
// consumer pact's $.metadata.trace_id type matcher is now satisfied by data of
// the real form.
func producerTraceMetadata() map[string]string {
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	return map[string]string{
		"trace_id": sc.TraceID().String(),
		"span_id":  sc.SpanID().String(),
	}
}

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
//
// This struct mirrors alt-backend's real producer type
// (shared/driver/mqhub_connect.ArticleCreatedPayload) field for field, including
// PublishedAt as time.Time (not string): the whole point of these fixtures is to
// verify the pact against what the producer actually serializes, so a type drift
// here -- string vs time.Time -- would let the verification pass against a shape
// the producer never emits (CDC-1).
type ArticleCreatedPayload struct {
	ArticleID   string    `json:"article_id"`
	UserID      string    `json:"user_id"`
	FeedID      string    `json:"feed_id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Content     string    `json:"content,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	PublishedAt time.Time `json:"published_at"`
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
		PublishedAt: publishedAtFixture,
	}
	payloadJSON, _ := json.Marshal(payload)

	event, _ := domain.NewEvent(
		domain.EventTypeArticleCreated,
		"alt-backend",
		payloadJSON,
		producerTraceMetadata(),
	)
	return event
}

// buildTagGenerationRequestedEvent creates a TagGenerationRequested event.
// reply_to mirrors usecase.ReplyStreamPrefix + the correlation id, which is the
// stream GenerateTagsForArticle actually blocks on; tag-generator publishes its
// reply straight to whatever string arrives here.
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
			"reply_to":       "alt:replies:tags:corr-001",
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

// No mq-hub-as-consumer pact is declared here, for either stream mq-hub writes.
//
// alt:events:articles is read by search-indexer and pre-processor, and
// alt:events:tags is read by tag-generator; each contract belongs to the reader
// as consumer (pacts/search-indexer-mq-hub.json, pacts/pre-processor-mq-hub.json,
// pacts/tag-generator-mq-hub.json), and provider_test.go replays all three
// against the event builders in this file. Declaring mq-hub as the consumer
// inverted the direction: such a pact could only ever be "verified" by mq-hub
// asserting on its own output, and having no provider-side verifier is what
// pushed the pair onto the manual-verification bridge in the first place.

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
