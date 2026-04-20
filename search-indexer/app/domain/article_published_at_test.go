package domain

import (
	"testing"
	"time"
)

// TestArticle_PublishedAt_Propagates ensures the domain carries the
// publication timestamp from ingestion through search indexing, so
// Meilisearch documents can be filtered by published_at window.
func TestArticle_PublishedAt_Propagates(t *testing.T) {
	published := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	article, err := NewArticleWithPublishedAt(
		"art-1",
		"Iran tensions escalate",
		"content body",
		[]string{"iran"},
		time.Date(2026, 4, 14, 12, 30, 0, 0, time.UTC),
		"user-1",
		published,
	)
	if err != nil {
		t.Fatalf("NewArticleWithPublishedAt() error = %v", err)
	}
	if !article.PublishedAt().Equal(published) {
		t.Errorf("PublishedAt() = %v, want %v", article.PublishedAt(), published)
	}
}

// TestSearchDocument_CarriesPublishedAt makes sure the SearchDocument shape
// used by the gateway / driver boundary exposes the publication date
// rather than dropping it, mirroring the earlier ``language`` regression.
func TestSearchDocument_CarriesPublishedAt(t *testing.T) {
	published := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	article, _ := NewArticleWithPublishedAt(
		"art-2",
		"a",
		"b",
		nil,
		time.Now(),
		"user-1",
		published,
	)
	doc := NewSearchDocument(article)
	if !doc.PublishedAt.Equal(published) {
		t.Errorf("SearchDocument.PublishedAt = %v, want %v", doc.PublishedAt, published)
	}
}
