//go:build contract

// Consumer-Driven Contract for pre-processor <- mq-hub on alt:events:articles.
//
// pre-processor is a claim-check consumer of this stream. It takes the article
// id off the event and lets the summarize job re-read the body from alt-db; it
// never reads an article body out of the payload. The interactions below
// therefore carry no `content` and no `tags`, which is what leaves the producer
// free to stop sending them (ADR-000953).
//
// A pact alone can only state "these fields must be present", so it cannot by
// itself stop pre-processor from growing a body dependency later.
// TestArticleStreamPayloadsCarryNoArticleBody closes that direction: it pins the
// exact JSON field set the two payload structs bind, and fails the moment a
// body field appears.
package contract

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/matchers"
	message "github.com/pact-foundation/pact-go/v2/message/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/consumer"
)

// redisStreamEvent is the wire shape mq-hub's RedisDriver.eventToValues()
// writes to the stream and consumer.parseEvent reads back.
type redisStreamEvent struct {
	EventID   string            `json:"event_id"`
	EventType string            `json:"event_type"`
	Source    string            `json:"source"`
	CreatedAt string            `json:"created_at"`
	Payload   json.RawMessage   `json:"payload"`
	Metadata  map[string]string `json:"metadata"`
}

// recordingSummarizer stands in for consumer.SummarizeServiceAdapter and
// captures the two arguments the handler extracts from the payload. Those two
// arguments are the whole of pre-processor's dependency on this stream.
type recordingSummarizer struct {
	called    bool
	articleID string
	title     string
}

func (r *recordingSummarizer) SummarizeArticle(_ context.Context, articleID, title string) error {
	r.called = true
	r.articleID = articleID
	r.title = title
	return nil
}

// newMqHubArticlesPact builds the pre-processor -> mq-hub asynchronous pact.
func newMqHubArticlesPact(t *testing.T) *message.AsynchronousPact {
	t.Helper()
	p, err := message.NewAsynchronousPact(message.Config{
		Consumer: "pre-processor",
		Provider: "mq-hub",
		PactDir:  pactDir,
	})
	require.NoError(t, err)
	return p
}

// decodeStreamEvent turns the pact-generated message into the consumer.Event
// that consumer.parseEvent would have produced from the Redis field map, and
// asserts the envelope properties pre-processor relies on along the way.
// parseEvent itself is unexported and takes a redis.XMessage, so the contract
// test reconstructs the same value from the wire JSON.
func decodeStreamEvent(t *testing.T, contents []byte) (redisStreamEvent, consumer.Event) {
	t.Helper()

	var wire redisStreamEvent
	require.NoError(t, json.Unmarshal(contents, &wire))

	assert.NotEmpty(t, wire.EventID, "event_id is logged on every handled event")
	assert.NotEmpty(t, wire.Source, "source is bound by parseEvent")

	// parseEvent uses time.Parse(time.RFC3339, ...) and drops the error, so a
	// format change would silently zero CreatedAt rather than fail loudly.
	createdAt, err := time.Parse(time.RFC3339, wire.CreatedAt)
	assert.NoError(t, err, "created_at must be parseable with time.RFC3339")

	return wire, consumer.Event{
		MessageID: "1742947200000-0",
		EventID:   wire.EventID,
		EventType: wire.EventType,
		Source:    wire.Source,
		CreatedAt: createdAt,
		Payload:   wire.Payload,
		Metadata:  wire.Metadata,
	}
}

// assertPayloadCarriesNoArticleBody fails if an interaction in this file ever
// starts expecting an article body. Widening the contract that way has to be a
// deliberate edit to this guard, not a quiet addition to a matcher map.
func assertPayloadCarriesNoArticleBody(t *testing.T, payload json.RawMessage) {
	t.Helper()

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &fields))

	for _, key := range []string{"content", "tags"} {
		_, present := fields[key]
		assert.Falsef(t, present, "pre-processor must not depend on %q; it re-reads the body via article_id (ADR-000953)", key)
	}
}

// TestConsumeArticleCreatedIDOnlyEvent pins the ArticleCreated interaction that
// drives event-driven summarization. Only article_id and title appear: the
// handler binds user_id / feed_id / url / published_at too, but never reads
// them, and pinning unread fields is how a producer ends up obliged to keep
// shipping data nobody consumes.
func TestConsumeArticleCreatedIDOnlyEvent(t *testing.T) {
	p := newMqHubArticlesPact(t)
	summarizer := &recordingSummarizer{}
	handler := consumer.NewPreProcessorEventHandler(summarizer, nil)

	err := p.AddAsynchronousMessage().
		Given("the articles stream exists").
		ExpectsToReceive("an ArticleCreated event on alt:events:articles").
		WithJSONContent(matchers.MapMatcher{
			"event_id":   matchers.Like("evt-uuid-001"),
			"event_type": matchers.String("ArticleCreated"),
			"source":     matchers.Like("alt-backend"),
			"created_at": matchers.Like("2026-03-26T00:00:00.000Z"),
			"payload": matchers.Like(matchers.MapMatcher{
				"article_id": matchers.Like("art-001"),
				"title":      matchers.Like("Breaking: Go 1.26 Released"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"trace_id": matchers.Like("trace-001"),
			}),
		}).
		AsType(&redisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			wire, event := decodeStreamEvent(t, contents.Contents)
			assert.Equal(t, "ArticleCreated", wire.EventType)
			assertPayloadCarriesNoArticleBody(t, wire.Payload)

			require.NoError(t, handler.HandleEvent(context.Background(), event))

			require.True(t, summarizer.called, "ArticleCreated must reach SummarizeArticle")
			assert.Equal(t, "art-001", summarizer.articleID, "article_id is the claim check pre-processor dereferences")
			assert.Equal(t, "Breaking: Go 1.26 Released", summarizer.title)
			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

// TestConsumeSummarizeRequestedIDOnlyEvent pins the second event type
// pre-processor handles on this stream. Its payload also carries `streaming`,
// which the handler logs but never acts on, so it stays out of the contract.
func TestConsumeSummarizeRequestedIDOnlyEvent(t *testing.T) {
	p := newMqHubArticlesPact(t)
	summarizer := &recordingSummarizer{}
	handler := consumer.NewPreProcessorEventHandler(summarizer, nil)

	err := p.AddAsynchronousMessage().
		Given("the articles stream exists").
		ExpectsToReceive("a SummarizeRequested event on alt:events:articles").
		WithJSONContent(matchers.MapMatcher{
			"event_id":   matchers.Like("evt-uuid-sum-001"),
			"event_type": matchers.String("SummarizeRequested"),
			"source":     matchers.Like("alt-backend"),
			"created_at": matchers.Like("2026-03-26T00:00:00.000Z"),
			"payload": matchers.Like(matchers.MapMatcher{
				"article_id": matchers.Like("art-002"),
				"title":      matchers.Like("Rust Memory Safety"),
			}),
			"metadata": matchers.Like(matchers.MapMatcher{
				"trace_id": matchers.Like("trace-002"),
			}),
		}).
		AsType(&redisStreamEvent{}).
		ConsumedBy(func(contents message.AsynchronousMessage) error {
			wire, event := decodeStreamEvent(t, contents.Contents)
			assert.Equal(t, "SummarizeRequested", wire.EventType)
			assertPayloadCarriesNoArticleBody(t, wire.Payload)

			require.NoError(t, handler.HandleEvent(context.Background(), event))

			require.True(t, summarizer.called, "SummarizeRequested must reach SummarizeArticle")
			assert.Equal(t, "art-002", summarizer.articleID)
			assert.Equal(t, "Rust Memory Safety", summarizer.title)
			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

// TestArticleStreamPayloadsCarryNoArticleBody is the consumer-side half of the
// guarantee the pacts above encode. The pacts say what mq-hub must send; this
// says what pre-processor is allowed to bind. Adding a `content` field to
// either payload struct fails here, which is the signal that the id-only pacts
// above no longer describe the service.
func TestArticleStreamPayloadsCarryNoArticleBody(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{
			name: "ArticleCreatedPayload",
			typ:  reflect.TypeOf(consumer.ArticleCreatedPayload{}),
			want: []string{"article_id", "user_id", "feed_id", "title", "url", "published_at"},
		},
		{
			name: "SummarizeRequestedPayload",
			typ:  reflect.TypeOf(consumer.SummarizeRequestedPayload{}),
			want: []string{"article_id", "user_id", "title", "streaming"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jsonFieldNames(tt.typ),
				"payload bound off alt:events:articles changed; the id-only pacts in this file must be revisited (ADR-000953)")
		})
	}
}

// jsonFieldNames returns the JSON keys a struct type binds, in declaration
// order, with any tag options (",omitempty") stripped.
func jsonFieldNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		names = append(names, name)
	}
	return names
}
