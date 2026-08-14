package articles

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	articlesv2 "alt/gen/proto/alt/articles/v2"

	"alt/config"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/utils/logger"
)

// sumWarmCounter totals every int64 Sum data point recorded under name,
// regardless of attributes, across an already-Collect()ed ResourceMetrics.
func sumWarmCounter(rm metricdata.ResourceMetrics, name string) int64 {
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	return total
}

// Bounding the warm pool (see TestBatchPrefetchImages_CacheWarmingIsBounded) is
// only half the job: the shed warms are real work thrown away, and production
// runs above debug level, so a saturated pool used to look exactly like a quiet
// one — the response is intact, the log is silent, and the only symptom is
// images that never warm. Every other shed/degrade path in this codebase is
// loud (outboxReleaseFailedCounter, host_rate_limiter.degraded_to_local,
// logImageProxyWiringState), so this one must be too.
//
// The started counter is what makes the shed counter readable: "shed 0" alone
// cannot distinguish a pool that never filled from a batch that had nothing to
// warm.
func TestBatchPrefetchImages_ShedWarmIsObservable(t *testing.T) {
	logger.InitLogger()

	reader := sdkmetric.NewManualReader()
	prevProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(prevProvider) })

	// Info level is what production runs at: a DebugContext line is invisible
	// there, which is the whole defect.
	var logs bytes.Buffer
	handlerLogger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cache := &blockingImageCache{release: make(chan struct{})}
	t.Cleanup(func() { close(cache.release) })

	handler := NewHandler(ArticleHandlerDeps{
		OgImageURLs: stubOgImageURLs{},
		ImageProxy: image_proxy_usecase.NewImageProxyUsecase(
			nil, nil, cache, stubImageSigner{}, nil, nil, 0, 0, 0),
	}, &config.Config{}, handlerLogger)
	ctx := createAuthContext()

	const (
		callCount  = 10
		idsPerCall = 10 // the handler's own per-RPC truncation
		attempted  = callCount * idsPerCall
	)

	for call := range callCount {
		articleIDs := make([]string, 0, idsPerCall)
		for i := range idsPerCall {
			articleIDs = append(articleIDs, fmt.Sprintf("article-%d-%d", call, i))
		}
		_, err := handler.BatchPrefetchImages(ctx,
			connect.NewRequest(&articlesv2.BatchPrefetchImagesRequest{ArticleIds: articleIDs}))
		require.NoError(t, err)
	}

	// Nothing can release a slot while the cache is parked, so the split
	// between claimed and shed is decided synchronously inside the RPCs and
	// there is nothing to wait for.
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	assert.Equal(t, int64(maxConcurrentCacheWarms),
		sumWarmCounter(rm, "alt_backend_image_cache_warm_started_total"),
		"warms that claimed a slot must be counted, or a shed rate has no denominator")
	assert.Equal(t, int64(attempted-maxConcurrentCacheWarms),
		sumWarmCounter(rm, "alt_backend_image_cache_warm_shed_total"),
		"every dropped warm must reach a counter; %d of %d were thrown away", attempted-maxConcurrentCacheWarms, attempted)

	// Loud in the log too, but throttled: one line per shed URL would be
	// hundreds per second exactly when the log is least readable.
	shedLines := strings.Count(logs.String(), "image_cache_warm.shed")
	assert.Equal(t, 1, shedLines,
		"the shed warning must be emitted once per window with an occurrence count, not once per dropped warm")
	assert.Contains(t, logs.String(), `"level":"WARN"`,
		"a saturated warm pool must be visible above debug level")
}
