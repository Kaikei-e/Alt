package fetch_feed_gateway

import (
	"alt/domain"

	"alt/utils/logger"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
)

const testRSSFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>Example Feed</title>
<link>https://example.com</link>
<description>Example description</description>
<item><title>Item One</title><link>https://example.com/1</link></item>
</channel></rss>`

// gzipBombFeed returns a well-formed RSS document — so feed type detection
// succeeds and the parser keeps reading — that is tiny on the wire but expands to
// more than maxFeedBodyBytes once the transport transparently decompresses it.
func gzipBombFeed(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := io.WriteString(zw, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Bomb</title><description>`); err != nil {
		t.Fatalf("write gzip prologue failed: %v", err)
	}
	chunk := bytes.Repeat([]byte("A"), 64*1024)
	for written := 0; written < maxFeedBodyBytes*2; written += len(chunk) {
		if _, err := zw.Write(chunk); err != nil {
			t.Fatalf("write gzip chunk failed: %v", err)
		}
	}
	if _, err := io.WriteString(zw, `</description></channel></rss>`); err != nil {
		t.Fatalf("write gzip epilogue failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip writer failed: %v", err)
	}
	return buf.Bytes()
}

func TestFetchFeedsGateway_OversizedFeedBody_Rejected(t *testing.T) {
	bomb := gzipBombFeed(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(bomb)
	}))
	defer server.Close()

	g := &FetchFeedsGateway{httpClient: server.Client()}

	items, err := g.FetchFeeds(context.Background(), server.URL+"/rss")
	if err == nil {
		t.Fatalf("expected error for oversized feed body, got %d items", len(items))
	}
}

func TestFetchFeedsGateway_NormalFeed_Fetched(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, testRSSFeed)
	}))
	defer server.Close()

	g := &FetchFeedsGateway{httpClient: server.Client()}

	items, err := g.FetchFeeds(context.Background(), server.URL+"/rss")
	if err != nil {
		t.Fatalf("expected no error for a normal feed, got %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Item One" {
		t.Errorf("expected 'Item One', got %q", items[0].Title)
	}
}

// newSingleFeedGatewayFor builds a SingleFeedGateway whose only subscription is
// feedURL. SingleFeedGateway builds its own client from the unified HTTP client
// factory, whose secure dialer refuses private addresses unless the host is
// operator-allow-listed via FEED_ALLOWED_HOSTS — httptest binds to loopback.
func newSingleFeedGatewayFor(t *testing.T, feedURL string) *SingleFeedGateway {
	t.Helper()

	logger.InitLogger()
	t.Setenv("FEED_ALLOWED_HOSTS", "127.0.0.1")

	return &SingleFeedGateway{
		store:       &pollableFeedLinkStoreStub{links: []domain.FeedLink{{ID: uuid.New(), URL: feedURL}}},
		rateLimiter: nil,
	}
}

func TestSingleFeedGateway_OversizedFeedBody_Rejected(t *testing.T) {
	bomb := gzipBombFeed(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(bomb)
	}))
	defer server.Close()

	g := newSingleFeedGatewayFor(t, server.URL+"/rss")

	feed, err := g.FetchSingleFeed(context.Background())
	if err == nil {
		t.Fatalf("expected error for oversized feed body, got feed %q", feed.Title)
	}
	if !errors.Is(err, gofeed.ErrResponseTooLarge) {
		t.Errorf("expected gofeed.ErrResponseTooLarge, got: %v", err)
	}
}

func TestSingleFeedGateway_NormalFeed_Fetched(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, testRSSFeed)
	}))
	defer server.Close()

	g := newSingleFeedGatewayFor(t, server.URL+"/rss")

	feed, err := g.FetchSingleFeed(context.Background())
	if err != nil {
		t.Fatalf("expected no error for a normal feed, got %v", err)
	}
	if feed.Title != "Example Feed" {
		t.Errorf("expected 'Example Feed', got %q", feed.Title)
	}
	if len(feed.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(feed.Items))
	}
}
