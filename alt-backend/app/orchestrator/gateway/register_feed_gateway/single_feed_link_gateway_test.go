package register_feed_gateway

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mmcdole/gofeed"
)

const testRSSFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>Example Feed</title>
<link>https://example.com</link>
<description>Example description</description>
<item><title>Item One</title><link>https://example.com/1</link></item>
</channel></rss>`

// newFeedServer serves a valid RSS feed over httptest (i.e. on a loopback address).
func newFeedServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, testRSSFeed)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestDefaultRSSFeedFetcher_NormalFeed_Fetched(t *testing.T) {
	server := newFeedServer(t)

	fetcher := newDefaultRSSFeedFetcher(true)
	feed, err := fetcher.FetchRSSFeed(context.Background(), server.URL+"/rss")
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

func TestDefaultRSSFeedFetcher_RedirectToInternalHost_Blocked(t *testing.T) {
	// Stands in for an internal service (e.g. the Kratos admin API) that is
	// reachable from the container but must never be reachable through a
	// registered feed URL. It serves a parseable feed, so a fetcher that follows
	// the redirect without re-validating succeeds instead of failing.
	internal := newFeedServer(t)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/admin", http.StatusFound)
	}))
	defer redirector.Close()

	fetcher := newDefaultRSSFeedFetcher(true)
	feed, err := fetcher.FetchRSSFeed(context.Background(), redirector.URL+"/rss")
	if err == nil {
		t.Fatalf("expected redirect to an internal host to be refused, got feed %q", feed.Title)
	}
	if !strings.Contains(err.Error(), "redirect blocked") {
		t.Errorf("expected an SSRF redirect error, got: %v", err)
	}
}

func TestDefaultRSSFeedFetcher_RedirectToAllowedHost_Followed(t *testing.T) {
	// FEED_ALLOWED_HOSTS is the operator's explicit trust decision — the first hop
	// honours it (URLSecurityValidator.isPrivateNetwork), so redirects must too.
	// It is live in compose.staging.yaml and in the alt-perf composite test.
	t.Setenv("FEED_ALLOWED_HOSTS", "127.0.0.1")

	allowed := newFeedServer(t)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, allowed.URL+"/rss", http.StatusFound)
	}))
	defer redirector.Close()

	fetcher := newDefaultRSSFeedFetcher(true)
	feed, err := fetcher.FetchRSSFeed(context.Background(), redirector.URL+"/rss")
	if err != nil {
		t.Fatalf("expected redirect to an allow-listed host to be followed, got %v", err)
	}
	if feed.Title != "Example Feed" {
		t.Errorf("expected 'Example Feed', got %q", feed.Title)
	}
}

func TestDefaultRSSFeedFetcher_DialToAllowlistedPrivateHost_Allowed(t *testing.T) {
	// Regression (E2E alt-backend shard / staging): the fetcher points at a feed
	// host that operators put on FEED_ALLOWED_HOSTS *because* it resolves to a
	// private/loopback address — the deps stub answers only for stub.invalid, which
	// maps to a container-internal IP (compose.staging.yaml:1165). The dial-time
	// SSRF guard must honour that allow-list, exactly as CheckRedirect and
	// URLSecurityValidator.isPrivateNetwork already do, or internal feed collection
	// is blocked. Uses the PRODUCTION fetcher (allowLoopbackDial = false) so the
	// only thing that can let the loopback dial through is the allow-list branch —
	// which is precisely what the old Dialer.Control guard could not see.
	server := newFeedServer(t)

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	// The httptest server binds to a loopback IP; trust exactly that host.
	t.Setenv("FEED_ALLOWED_HOSTS", u.Hostname())

	fetcher := NewDefaultRSSFeedFetcher()
	feed, err := fetcher.FetchRSSFeed(context.Background(), server.URL+"/rss")
	if err != nil {
		t.Fatalf("expected an allow-listed private/loopback host to be dialable, got %v", err)
	}
	if feed.Title != "Example Feed" {
		t.Errorf("expected 'Example Feed', got %q", feed.Title)
	}
}

func TestDefaultRSSFeedFetcher_DialToNonAllowlistedPrivateIP_Blocked(t *testing.T) {
	// The allow-list escape hatch must not weaken SSRF protection for anything
	// else: a private IP that is NOT on FEED_ALLOWED_HOSTS must still be refused at
	// connection time. Production fetcher, empty allow-list.
	t.Setenv("FEED_ALLOWED_HOSTS", "")

	fetcher := NewDefaultRSSFeedFetcher()
	_, err := fetcher.FetchRSSFeed(context.Background(), "http://10.0.0.1/rss")
	if err == nil {
		t.Fatal("expected a non-allow-listed private IP to be blocked at dial time, got nil")
	}
	if !strings.Contains(err.Error(), "PRIVATE_IP_BLOCKED") {
		t.Errorf("expected a PRIVATE_IP_BLOCKED SSRF error, got: %v", err)
	}
}

func TestDefaultRSSFeedFetcher_DialToMetadataIP_Blocked(t *testing.T) {
	// Production fetcher (loopback dialing NOT allowed). A feed URL whose host is
	// a cloud-metadata / link-local IP must be refused at connection time by the
	// Dialer.Control SSRF guard, closing the DNS-rebinding window that pre-fetch
	// URL validation alone leaves open. The error must be the SSRF policy block,
	// not an incidental network failure.
	fetcher := NewDefaultRSSFeedFetcher()
	_, err := fetcher.FetchRSSFeed(context.Background(), "http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("expected connection-time SSRF block for a metadata/link-local IP, got nil")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected an SSRF connection-time block error, got: %v", err)
	}
}

func TestDefaultRSSFeedFetcher_OversizedFeedBody_Rejected(t *testing.T) {
	// A well-formed RSS document — so feed type detection succeeds and the parser
	// keeps reading — that is tiny on the wire but huge once the transport
	// transparently decompresses it.
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
	bomb := buf.Bytes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(bomb)
	}))
	defer server.Close()

	fetcher := newDefaultRSSFeedFetcher(true)
	_, err := fetcher.FetchRSSFeed(context.Background(), server.URL+"/rss")
	if err == nil {
		t.Fatal("expected error for oversized feed body, got nil")
	}
	if !errors.Is(err, gofeed.ErrResponseTooLarge) {
		t.Errorf("expected gofeed.ErrResponseTooLarge, got: %v", err)
	}
}
