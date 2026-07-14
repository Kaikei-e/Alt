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
	headsErr, imagesErr, expiredErr error
	calls                           []string
	headTTL, imageTTL               time.Duration
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
	p := &mockRetentionPurger{heads: 3, images: 5, expired: 2}

	fn := ogImageRetentionJobFn(p)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := []string{"heads", "images", "expired"}
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
}

func TestOgImageRetentionJob_PropagatesArticleHeadError(t *testing.T) {
	p := &mockRetentionPurger{headsErr: errors.New("boom")}

	fn := ogImageRetentionJobFn(p)
	if err := fn(context.Background()); err == nil {
		t.Fatal("expected error when article_heads purge fails, got nil")
	}
}

func TestOgImageRetentionJob_PropagatesImageCacheError(t *testing.T) {
	p := &mockRetentionPurger{imagesErr: errors.New("boom")}

	fn := ogImageRetentionJobFn(p)
	if err := fn(context.Background()); err == nil {
		t.Fatal("expected error when image cache purge fails, got nil")
	}
}
