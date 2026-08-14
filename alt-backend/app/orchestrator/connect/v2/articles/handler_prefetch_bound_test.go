package articles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	articlesv2 "alt/gen/proto/alt/articles/v2"

	"alt/config"
	"alt/domain"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/utils/logger"
)

// blockingImageCache parks every warm inside GetCachedImage — the first thing
// WarmCache does — so the test can count how many warms are alive at once
// without racing their completion.
type blockingImageCache struct {
	started atomic.Int64
	release chan struct{}
}

func (c *blockingImageCache) GetCachedImage(_ context.Context, _ string) (*domain.ImageProxyCacheEntry, error) {
	c.started.Add(1)
	<-c.release
	return nil, errors.New("cache unavailable")
}

func (c *blockingImageCache) SaveCachedImage(_ context.Context, _ *domain.ImageProxyCacheEntry) error {
	return nil
}

func (c *blockingImageCache) CleanupExpiredImages(_ context.Context) (int64, error) { return 0, nil }

// stubImageSigner mints proxy URLs in the shape WarmCache parses, and refuses
// to verify them so the warm stops before it would need an HTTP client.
type stubImageSigner struct{}

func (stubImageSigner) GenerateProxyURL(imageURL string) string {
	return "/v1/images/proxy/sig/" + imageURL
}

func (stubImageSigner) VerifyAndDecode(_, _ string) (string, error) {
	return "", errors.New("not verifiable in test")
}

// stubOgImageURLs hands every requested article a distinct image URL, so one
// RPC is worth as many warms as it has article ids.
type stubOgImageURLs struct{}

func (stubOgImageURLs) FetchOgImageURLsByArticleIDs(_ context.Context, articleIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(articleIDs))
	for _, id := range articleIDs {
		out[id] = "https://example.com/" + id + ".jpg"
	}
	return out, nil
}

// BatchPrefetchImages detaches its cache warming with context.WithoutCancel and
// a 60-second budget, so a warm outlives the RPC that asked for it. The Connect
// listener has no rate limit of its own and WarmCache parks on a per-host
// limiter, so without a ceiling the live goroutine count tracks the arrival
// rate rather than the completion rate — a few hundred rps is tens of thousands
// of goroutines holding a request context each.
func TestBatchPrefetchImages_CacheWarmingIsBounded(t *testing.T) {
	logger.InitLogger()

	cache := &blockingImageCache{release: make(chan struct{})}
	t.Cleanup(func() { close(cache.release) })

	handler := NewHandler(ArticleHandlerDeps{
		OgImageURLs: stubOgImageURLs{},
		ImageProxy: image_proxy_usecase.NewImageProxyUsecase(
			nil, nil, cache, stubImageSigner{}, nil, nil, 0, 0, 0),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	const (
		callCount      = 40
		idsPerCall     = 10 // the handler's own per-RPC truncation
		wouldBeStarted = callCount * idsPerCall
	)

	for call := range callCount {
		articleIDs := make([]string, 0, idsPerCall)
		for i := range idsPerCall {
			articleIDs = append(articleIDs, fmt.Sprintf("article-%d-%d", call, i))
		}

		resp, err := handler.BatchPrefetchImages(ctx,
			connect.NewRequest(&articlesv2.BatchPrefetchImagesRequest{ArticleIds: articleIDs}))
		require.NoError(t, err)
		require.Len(t, resp.Msg.Images, idsPerCall,
			"shedding a warm must not shrink the response the caller asked for")
	}

	// Nothing can finish while the cache is parked, so every warm that ever
	// started is still alive. Give the scheduler a moment to start them.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cache.started.Load() > maxConcurrentCacheWarms {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	require.LessOrEqual(t, cache.started.Load(), int64(maxConcurrentCacheWarms),
		"%d requests fanned out unbounded (up to %d live warms), each holding a request context for 60s",
		callCount, wouldBeStarted)
}
