package job

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// mockUnwarmedFetcher implements ogpUnwarmedFetcher for testing.
type mockUnwarmedFetcher struct {
	urls []string
	err  error
}

func (m *mockUnwarmedFetcher) FetchUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error) {
	return m.urls, m.err
}

// mockImageWarmer implements imageWarmer for testing.
type mockImageWarmer struct {
	mu      sync.Mutex
	warmed  []string
	failFor map[string]bool // URLs that should "fail" (WarmCache is fire-and-forget, so we just track calls)
}

func (m *mockImageWarmer) WarmCache(ctx context.Context, imageURL string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warmed = append(m.warmed, imageURL)
}

func (m *mockImageWarmer) GenerateProxyURL(imageURL string) string {
	if imageURL == "" {
		return ""
	}
	return "/v1/images/proxy/sig/" + imageURL
}

func TestOgpImageWarmerJob_NoURLs(t *testing.T) {
	fetcher := &mockUnwarmedFetcher{urls: nil}
	warmer := &mockImageWarmer{}

	fn := ogpImageWarmerJobFn(fetcher, warmer)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(warmer.warmed) != 0 {
		t.Fatalf("expected 0 warmed URLs, got %d", len(warmer.warmed))
	}
}

func TestOgpImageWarmerJob_WarmsAllURLs(t *testing.T) {
	urls := []string{
		"https://example.com/img1.jpg",
		"https://example.com/img2.png",
		"https://other.com/img3.webp",
	}
	fetcher := &mockUnwarmedFetcher{urls: urls}
	warmer := &mockImageWarmer{}

	fn := ogpImageWarmerJobFn(fetcher, warmer)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(warmer.warmed) != 3 {
		t.Fatalf("expected 3 warmed URLs, got %d", len(warmer.warmed))
	}
	for i, url := range urls {
		if warmer.warmed[i] != url {
			t.Errorf("warmed[%d] = %q, want %q", i, warmer.warmed[i], url)
		}
	}
}

func TestOgpImageWarmerJob_FetchError(t *testing.T) {
	fetcher := &mockUnwarmedFetcher{err: errors.New("db error")}
	warmer := &mockImageWarmer{}

	fn := ogpImageWarmerJobFn(fetcher, warmer)
	err := fn(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "fetch unwarmed og image URLs: db error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOgpImageWarmerJob_ContextCancelled(t *testing.T) {
	urls := []string{
		"https://example.com/img1.jpg",
		"https://example.com/img2.png",
	}
	fetcher := &mockUnwarmedFetcher{urls: urls}
	warmer := &mockImageWarmer{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	fn := ogpImageWarmerJobFn(fetcher, warmer)
	err := fn(ctx)
	// Should return early without error when context is cancelled
	if err != nil {
		t.Fatalf("expected nil error on cancelled context, got %v", err)
	}
	// Should not have warmed anything (or at most partially)
	if len(warmer.warmed) > 0 {
		t.Logf("warmed %d URLs before context cancellation (acceptable)", len(warmer.warmed))
	}
}

func TestOgpImageWarmerJob_SkipsEmptyURLs(t *testing.T) {
	urls := []string{
		"https://example.com/img1.jpg",
		"", // empty - should skip
		"https://example.com/img3.jpg",
	}
	fetcher := &mockUnwarmedFetcher{urls: urls}
	warmer := &mockImageWarmer{}

	fn := ogpImageWarmerJobFn(fetcher, warmer)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(warmer.warmed) != 2 {
		t.Fatalf("expected 2 warmed URLs (skipping empty), got %d", len(warmer.warmed))
	}
}
