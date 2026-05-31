package job

import (
	"alt/driver/alt_db"
	"context"
	"errors"
	"sync"
	"testing"
)

type mockCandidateLister struct {
	candidates []alt_db.OgBackfillCandidate
	err        error
}

func (m *mockCandidateLister) FetchFeedsMissingOgImage(ctx context.Context, limit int) ([]alt_db.OgBackfillCandidate, error) {
	return m.candidates, m.err
}

type mockArticleContentFetcher struct {
	mu       sync.Mutex
	byURL    map[string]string
	errByURL map[string]error
	fetched  []string
}

func (m *mockArticleContentFetcher) FetchArticleContents(ctx context.Context, articleURL string) (*string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetched = append(m.fetched, articleURL)
	if err := m.errByURL[articleURL]; err != nil {
		return nil, err
	}
	if html, ok := m.byURL[articleURL]; ok {
		return &html, nil
	}
	empty := ""
	return &empty, nil
}

type mockArticleHeadSaver struct {
	mu    sync.Mutex
	saved map[string]string // articleID -> ogImageURL
}

func (m *mockArticleHeadSaver) SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saved == nil {
		m.saved = map[string]string{}
	}
	m.saved[articleID] = ogImageURL
	return nil
}

func pageWithOgImage(img string) string {
	return `<html><head><meta property="og:image" content="` + img + `" /></head><body>hi</body></html>`
}

func TestOgImageBackfillJob_ScrapesAndStoresOgImage(t *testing.T) {
	lister := &mockCandidateLister{candidates: []alt_db.OgBackfillCandidate{
		{ArticleID: "a1", URL: "https://example.com/1"},
		{ArticleID: "a2", URL: "https://example.com/2"},
	}}
	fetcher := &mockArticleContentFetcher{byURL: map[string]string{
		"https://example.com/1": pageWithOgImage("https://cdn.example.com/1.jpg"),
		"https://example.com/2": pageWithOgImage("https://cdn.example.com/2.jpg"),
	}}
	saver := &mockArticleHeadSaver{}
	warmer := &mockImageWarmer{}

	fn := ogImageBackfillJobFn(lister, fetcher, saver, warmer)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if saver.saved["a1"] != "https://cdn.example.com/1.jpg" {
		t.Errorf("a1 og image = %q, want cdn 1.jpg", saver.saved["a1"])
	}
	if saver.saved["a2"] != "https://cdn.example.com/2.jpg" {
		t.Errorf("a2 og image = %q, want cdn 2.jpg", saver.saved["a2"])
	}
	if len(warmer.warmed) != 2 {
		t.Errorf("expected 2 warmed images, got %d", len(warmer.warmed))
	}
}

func TestOgImageBackfillJob_SkipsWhenNoOgImageFound(t *testing.T) {
	lister := &mockCandidateLister{candidates: []alt_db.OgBackfillCandidate{
		{ArticleID: "a1", URL: "https://example.com/1"},
	}}
	fetcher := &mockArticleContentFetcher{byURL: map[string]string{
		"https://example.com/1": `<html><head><title>no image</title></head><body>hi</body></html>`,
	}}
	saver := &mockArticleHeadSaver{}
	warmer := &mockImageWarmer{}

	fn := ogImageBackfillJobFn(lister, fetcher, saver, warmer)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(saver.saved) != 0 {
		t.Errorf("expected nothing saved when no og:image, got %v", saver.saved)
	}
}

func TestOgImageBackfillJob_ContinuesPastFetchError(t *testing.T) {
	lister := &mockCandidateLister{candidates: []alt_db.OgBackfillCandidate{
		{ArticleID: "a1", URL: "https://example.com/1"},
		{ArticleID: "a2", URL: "https://example.com/2"},
	}}
	fetcher := &mockArticleContentFetcher{
		errByURL: map[string]error{"https://example.com/1": errors.New("403")},
		byURL:    map[string]string{"https://example.com/2": pageWithOgImage("https://cdn.example.com/2.jpg")},
	}
	saver := &mockArticleHeadSaver{}
	warmer := &mockImageWarmer{}

	fn := ogImageBackfillJobFn(lister, fetcher, saver, warmer)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error despite per-article fetch failure, got %v", err)
	}
	if _, ok := saver.saved["a1"]; ok {
		t.Errorf("a1 should be skipped on fetch error")
	}
	if saver.saved["a2"] != "https://cdn.example.com/2.jpg" {
		t.Errorf("a2 should still be backfilled, got %q", saver.saved["a2"])
	}
}

func TestOgImageBackfillJob_FetchListError(t *testing.T) {
	lister := &mockCandidateLister{err: errors.New("db down")}
	fn := ogImageBackfillJobFn(lister, &mockArticleContentFetcher{}, &mockArticleHeadSaver{}, &mockImageWarmer{})
	if err := fn(context.Background()); err == nil {
		t.Fatal("expected error when listing candidates fails")
	}
}
