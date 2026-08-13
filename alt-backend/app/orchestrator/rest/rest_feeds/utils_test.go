package rest_feeds

import (
	"testing"
	"time"

	"alt/domain"
)

// TestDeriveNextCursorFromFeeds_KeepsSubSecondPrecision pins the pagination
// boundary: created_at is microsecond precision and one harvester transaction
// stamps a whole batch inside the same second, so a cursor that only names the
// second is fed back into `created_at < $1` and drops the rest of that second.
func TestDeriveNextCursorFromFeeds_KeepsSubSecondPrecision(t *testing.T) {
	ts := time.Date(2026, time.March, 2, 10, 0, 0, 123456000, time.UTC)

	cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{{PublishedParsed: ts}})
	if !ok {
		t.Fatalf("expected cursor to be derived")
	}

	parsed, err := time.Parse(time.RFC3339, cursor)
	if err != nil {
		t.Fatalf("cursor %q is not parseable by the cursor parse side: %v", cursor, err)
	}
	if !parsed.Equal(ts) {
		t.Fatalf("cursor lost precision: %s became %s", ts.Format(time.RFC3339Nano), parsed.Format(time.RFC3339Nano))
	}
}

func TestDeriveNextCursorFromFeeds_FallsBackToPublishedString(t *testing.T) {
	published := "2026-03-02T10:00:00.123456Z"

	cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{{Published: published}})
	if !ok {
		t.Fatalf("expected cursor to be derived from the published string")
	}
	if cursor != published {
		t.Fatalf("expected cursor %s, got %s", published, cursor)
	}
}
