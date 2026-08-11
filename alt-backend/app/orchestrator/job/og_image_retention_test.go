package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockRetentionPurger implements ogImageRetentionPurger for testing.
type mockRetentionPurger struct {
	heads, images, expired          int64
	feedImages                      int64
	headsErr, imagesErr, expiredErr error
	feedImagesErr                   error
	calls                           []string
	headTTL, imageTTL               time.Duration
	feedImageTTL                    time.Duration
}

func (m *mockRetentionPurger) CleanupExpiredFeedOgImages(ctx context.Context, ttl time.Duration) (int64, error) {
	m.calls = append(m.calls, "feed_images")
	m.feedImageTTL = ttl
	return m.feedImages, m.feedImagesErr
}

func (m *mockRetentionPurger) CleanupExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error) {
	m.calls = append(m.calls, "heads")
	m.headTTL = ttl
	return m.heads, m.headsErr
}

func (m *mockRetentionPurger) CleanupImageProxyCacheOlderThan(ctx context.Context, ttl time.Duration) (int64, error) {
	m.calls = append(m.calls, "images")
	m.imageTTL = ttl
	return m.images, m.imagesErr
}

func (m *mockRetentionPurger) CleanupExpiredImageProxyCache(ctx context.Context) (int64, error) {
	m.calls = append(m.calls, "expired")
	return m.expired, m.expiredErr
}

func TestOgImageRetentionJob_PurgesAllArtifactsWithin7DayWindow(t *testing.T) {
	p := &mockRetentionPurger{heads: 3, feedImages: 4, images: 5, expired: 2}

	fn := ogImageRetentionJobFn(p, p)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// feed_og_images joins the sweep with on-demand resolution. It holds the
	// same kind of artifact as article_heads — a scraped third-party image URL
	// — so a resolver that acquired these without a sweep to remove them would
	// be worse than not resolving at all.
	want := []string{"heads", "feed_images", "images", "expired"}
	if len(p.calls) != len(want) {
		t.Fatalf("expected purge calls %v, got %v", want, p.calls)
	}
	for i, c := range want {
		if p.calls[i] != c {
			t.Errorf("call[%d] = %q, want %q", i, p.calls[i], c)
		}
	}
	if p.headTTL != 7*24*time.Hour {
		t.Errorf("article head retention window = %v, want 7 days", p.headTTL)
	}
	if p.imageTTL != 7*24*time.Hour {
		t.Errorf("image cache retention window = %v, want 7 days", p.imageTTL)
	}
	if p.feedImageTTL != 7*24*time.Hour {
		t.Errorf("feed og image retention window = %v, want 7 days", p.feedImageTTL)
	}
}

func TestOgImageRetentionJob_PropagatesArticleHeadError(t *testing.T) {
	p := &mockRetentionPurger{headsErr: errors.New("boom")}

	fn := ogImageRetentionJobFn(p, p)
	if err := fn(context.Background()); err == nil {
		t.Fatal("expected error when article_heads purge fails, got nil")
	}
}

func TestOgImageRetentionJob_PropagatesImageCacheError(t *testing.T) {
	p := &mockRetentionPurger{imagesErr: errors.New("boom")}

	fn := ogImageRetentionJobFn(p, p)
	if err := fn(context.Background()); err == nil {
		t.Fatal("expected error when image cache purge fails, got nil")
	}
}
