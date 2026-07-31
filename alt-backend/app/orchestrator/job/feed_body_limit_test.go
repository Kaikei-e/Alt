package job

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
	"net/url"
	"testing"

	"github.com/google/uuid"
	rssFeed "github.com/mmcdole/gofeed"
)

const testRSSFeedBody = `<?xml version="1.0" encoding="UTF-8"?>
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

// newBombFeedServer serves the gzip bomb over httptest.
func newBombFeedServer(t *testing.T) *httptest.Server {
	t.Helper()

	bomb := gzipBombFeed(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(bomb)
	}))
	t.Cleanup(server.Close)
	return server
}

// newNormalFeedServer serves a small, valid RSS feed over httptest.
func newNormalFeedServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, testRSSFeedBody)
	}))
	t.Cleanup(server.Close)
	return server
}

// allowLoopbackFeedHost lets the unified HTTP client factory dial the httptest
// server: its secure dialer refuses private addresses unless the host is
// operator-allow-listed via FEED_ALLOWED_HOSTS.
func allowLoopbackFeedHost(t *testing.T) {
	t.Helper()

	t.Setenv("FEED_ALLOWED_HOSTS", "127.0.0.1")
}

func TestCollectSingleFeed_OversizedFeedBody_Rejected(t *testing.T) {
	logger.InitLogger()
	allowLoopbackFeedHost(t)
	server := newBombFeedServer(t)

	feedURL, err := url.Parse(server.URL + "/rss")
	if err != nil {
		t.Fatalf("parse feed url failed: %v", err)
	}

	feed, err := CollectSingleFeed(context.Background(), *feedURL, nil)
	if err == nil {
		t.Fatalf("expected error for oversized feed body, got feed %q", feed.Title)
	}
	if !errors.Is(err, rssFeed.ErrResponseTooLarge) {
		t.Errorf("expected gofeed.ErrResponseTooLarge, got: %v", err)
	}
}

func TestCollectSingleFeed_NormalFeed_Collected(t *testing.T) {
	logger.InitLogger()
	allowLoopbackFeedHost(t)
	server := newNormalFeedServer(t)

	feedURL, err := url.Parse(server.URL + "/rss")
	if err != nil {
		t.Fatalf("parse feed url failed: %v", err)
	}

	feed, err := CollectSingleFeed(context.Background(), *feedURL, nil)
	if err != nil {
		t.Fatalf("expected no error for a normal feed, got %v", err)
	}
	if feed.Title != "Example Feed" {
		t.Errorf("expected 'Example Feed', got %q", feed.Title)
	}
}

func TestCollectMultipleFeeds_OversizedFeedBody_Rejected(t *testing.T) {
	logger.InitLogger()
	allowLoopbackFeedHost(t)
	server := newBombFeedServer(t)

	feedLinks := []domain.FeedLink{{ID: uuid.New(), URL: server.URL + "/rss"}}

	items, err := CollectMultipleFeeds(context.Background(), feedLinks, nil, nil)
	if err == nil {
		t.Fatalf("expected error for oversized feed body, got %d items", len(items))
	}
	if !errors.Is(err, rssFeed.ErrResponseTooLarge) {
		t.Errorf("expected gofeed.ErrResponseTooLarge, got: %v", err)
	}
}

func TestCollectMultipleFeeds_NormalFeed_Collected(t *testing.T) {
	logger.InitLogger()
	allowLoopbackFeedHost(t)
	server := newNormalFeedServer(t)

	feedLinks := []domain.FeedLink{{ID: uuid.New(), URL: server.URL + "/rss"}}

	items, err := CollectMultipleFeeds(context.Background(), feedLinks, nil, nil)
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

func TestNewBoundedFeedParser_OversizedFeedBody_Rejected(t *testing.T) {
	server := newBombFeedServer(t)

	_, err := newBoundedFeedParser(server.Client()).ParseURL(server.URL + "/rss")
	if err == nil {
		t.Fatal("expected error for oversized feed body, got nil")
	}
	if !errors.Is(err, rssFeed.ErrResponseTooLarge) {
		t.Errorf("expected gofeed.ErrResponseTooLarge, got: %v", err)
	}
}

func TestNewBoundedFeedParser_NormalFeed_Parsed(t *testing.T) {
	server := newNormalFeedServer(t)

	feed, err := newBoundedFeedParser(server.Client()).ParseURL(server.URL + "/rss")
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
