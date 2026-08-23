package consumer

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

// TestParseEvent_MalformedFieldsDoNotAbort covers GO-7: a malformed created_at
// or metadata must not silently corrupt the event or halt parsing — the bad
// field is skipped (and logged) while the rest of the event is still parsed.
func TestParseEvent_MalformedFieldsDoNotAbort(t *testing.T) {
	c := &Consumer{logger: newQuietLogger()}

	msg := redis.XMessage{
		ID: "9-0",
		Values: map[string]interface{}{
			"event_id":   "e1",
			"event_type": "article.created",
			"created_at": "not-a-timestamp",
			"payload":    `{"article_id":"abc"}`,
			"metadata":   "{not json",
		},
	}

	event := c.parseEvent(msg)

	if event.EventID != "e1" {
		t.Fatalf("EventID = %q, want e1", event.EventID)
	}
	if event.EventType != "article.created" {
		t.Fatalf("EventType = %q, want article.created", event.EventType)
	}
	if !event.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt should stay zero on unparseable input, got %v", event.CreatedAt)
	}
	if string(event.Payload) != `{"article_id":"abc"}` {
		t.Fatalf("Payload = %q, want the raw JSON payload", string(event.Payload))
	}
	// Metadata was initialized and left empty rather than nil, so downstream map
	// reads are safe even when the metadata field is malformed.
	if event.Metadata == nil {
		t.Fatal("Metadata should be a non-nil (empty) map after a failed unmarshal")
	}
	if len(event.Metadata) != 0 {
		t.Fatalf("Metadata should be empty after a failed unmarshal, got %v", event.Metadata)
	}
}
