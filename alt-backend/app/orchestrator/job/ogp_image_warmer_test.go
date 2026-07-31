package job

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
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

// Finding [12]: unlike TagCloudCacheWarmerJob, ImageProxyUsecase IS
// legitimately nil when IMAGE_PROXY_ENABLED=false or the secret is empty
// (finding [6] already makes that state loud at DI-construction time via
// logImageProxyWiringState / a production-only panic there). So this job
// must not panic — but the per-tick skip log must be at least Warn level
// with an explicit "disabled" reason, not an easily-missed Info log that
// reads identically whether the feature is off on purpose or by DI mistake.
func TestOgpImageWarmerJob_NilImageProxy_LogsWarnWithDisabledReason(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(prev)

	// The fetcher is wired; only the image proxy is config-disabled. Those
	// are different states and ADR-000954 Wave 3 made them different code
	// paths: an unwired fetcher is a composition-root bug and panics
	// (see TestOgpImageWarmerJob_NilFetcherPanics), while a disabled image
	// proxy is a supported configuration and logs.
	fn := OgpImageWarmerJob(&mockUnwarmedFetcher{}, nil)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("expected WARN-level log for nil image proxy, got: %s", out)
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("expected an explicit 'disabled' reason in the log, got: %s", out)
	}
}

// TestOgpImageWarmerJob_NilFetcherPanics: the unwarmed-image query has no
// feature flag. A nil fetcher can only mean the composition root forgot to
// pass the data-hub gateway, and the job would then log "no unwarmed images
// found" on every tick — a healthy-looking line for a warmer that warms
// nothing (CLAUDE.md rule 8).
func TestOgpImageWarmerJob_NilFetcherPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for a nil unwarmed-image fetcher")
		}
	}()
	OgpImageWarmerJob(nil, nil)
}
