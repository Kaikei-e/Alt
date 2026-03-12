package feed_page_cache_gateway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"alt/driver/alt_db"

	"github.com/google/uuid"
)

type feedPageDBStub struct {
	loads int32
	rows  []*alt_db.FeedPageRow
}

func (s *feedPageDBStub) FetchFeedsByFeedLinkID(ctx context.Context, feedLinkID uuid.UUID) ([]*alt_db.FeedPageRow, error) {
	atomic.AddInt32(&s.loads, 1)
	return s.rows, nil
}

func TestGateway_GetFeedPage_UsesCache(t *testing.T) {
	feedLinkID := uuid.New()
	row := &alt_db.FeedPageRow{
		FeedID:    uuid.New(),
		Title:     "title",
		Link:      "https://example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db := &feedPageDBStub{rows: []*alt_db.FeedPageRow{row}}
	gateway := newGateway(db)

	first, err := gateway.GetFeedPage(context.Background(), feedLinkID)
	if err != nil {
		t.Fatalf("GetFeedPage() error = %v", err)
	}
	second, err := gateway.GetFeedPage(context.Background(), feedLinkID)
	if err != nil {
		t.Fatalf("GetFeedPage() second error = %v", err)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("unexpected feed page lengths: %d %d", len(first), len(second))
	}
	if atomic.LoadInt32(&db.loads) != 1 {
		t.Fatalf("loads = %d, want 1", db.loads)
	}
}

func TestGateway_LoadFeedPage_PreComputesFields(t *testing.T) {
	feedLinkID := uuid.New()
	feedID := uuid.New()
	now := time.Now()
	row := &alt_db.FeedPageRow{
		FeedID:      feedID,
		Title:       "title",
		Description: "<b>Bold</b> &amp; text",
		Link:        "https://example.com",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	db := &feedPageDBStub{rows: []*alt_db.FeedPageRow{row}}
	gateway := newGateway(db)

	entries, err := gateway.GetFeedPage(context.Background(), feedLinkID)
	if err != nil {
		t.Fatalf("GetFeedPage() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	entry := entries[0]
	// SanitizedDescription: HTML stripped and entities decoded
	if entry.SanitizedDescription != "Bold & text" {
		t.Fatalf("SanitizedDescription = %q, want %q", entry.SanitizedDescription, "Bold & text")
	}
	// FeedIDStr: pre-computed UUID string
	if entry.FeedIDStr != feedID.String() {
		t.Fatalf("FeedIDStr = %q, want %q", entry.FeedIDStr, feedID.String())
	}
	// PublishedStr: pre-computed RFC3339 string
	if entry.PublishedStr != now.Format(time.RFC3339) {
		t.Fatalf("PublishedStr = %q, want %q", entry.PublishedStr, now.Format(time.RFC3339))
	}
}

func TestGateway_InvalidateFeedPage(t *testing.T) {
	db := &feedPageDBStub{rows: []*alt_db.FeedPageRow{{FeedID: uuid.New()}}}
	gateway := newGateway(db)
	feedLinkID := uuid.New()

	if _, err := gateway.GetFeedPage(context.Background(), feedLinkID); err != nil {
		t.Fatalf("GetFeedPage() error = %v", err)
	}
	_ = gateway.InvalidateFeedPage(context.Background(), feedLinkID)
	if _, err := gateway.GetFeedPage(context.Background(), feedLinkID); err != nil {
		t.Fatalf("GetFeedPage() after invalidate error = %v", err)
	}
	if atomic.LoadInt32(&db.loads) != 2 {
		t.Fatalf("loads = %d, want 2", db.loads)
	}
}
