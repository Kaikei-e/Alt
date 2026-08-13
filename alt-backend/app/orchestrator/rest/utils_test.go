package rest

import (
	"alt/domain"
	"testing"
	"time"
)

func TestDeriveNextCursorFromFeeds(t *testing.T) {
	t.Run("returns false when feeds slice is empty", func(t *testing.T) {
		if cursor, ok := DeriveNextCursorFromFeeds(nil); ok || cursor != "" {
			t.Fatalf("expected no cursor, got %v (ok=%v)", cursor, ok)
		}
	})

	t.Run("uses PublishedParsed when available", func(t *testing.T) {
		ts := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
		cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{
			{
				Title:           "foo",
				PublishedParsed: ts,
			},
		})
		if !ok {
			t.Fatalf("expected cursor to be derived")
		}
		if cursor != ts.Format(time.RFC3339) {
			t.Fatalf("unexpected cursor %s", cursor)
		}
	})

	t.Run("falls back to parsing Published string", func(t *testing.T) {
		published := "2024-02-01T00:00:00Z"
		cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{
			{
				Title:     "bar",
				Published: published,
			},
		})
		if !ok {
			t.Fatalf("expected cursor to be derived from string")
		}
		if cursor != published {
			t.Fatalf("expected cursor %s, got %s", published, cursor)
		}
	})

	t.Run("returns false when published string is invalid", func(t *testing.T) {
		cursor, ok := DeriveNextCursorFromFeeds([]*domain.FeedItem{
			{
				Published: "invalid",
			},
		})
		if ok || cursor != "" {
			t.Fatalf("expected derive failure, got %v (ok=%v)", cursor, ok)
		}
	})
}

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
