package articles

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"alt/connect/errorhandler"
	"alt/domain"
	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/utils/security"
)

// These OTel counters close a specific gap: the shed used to be a
// DebugContext line and nothing else, so above debug level — which is where
// production runs — a saturated warm pool and a batch with nothing to warm
// were the same observation. Both leave the response whole and the log silent,
// and the only symptom is images that never warm.
//
// The started counter is what makes the shed counter readable: a zero shed
// count next to a zero started count means idle, next to a non-zero started
// count means healthy. Alert on the ratio, not the raw shed rate.
var (
	warmMeterOnce           sync.Once
	cacheWarmStartedCounter metric.Int64Counter
	cacheWarmShedCounter    metric.Int64Counter
)

func initCacheWarmMetrics() {
	warmMeterOnce.Do(func() {
		meter := otel.Meter("alt-backend.image-cache-warm")
		cacheWarmStartedCounter, _ = meter.Int64Counter("alt_backend_image_cache_warm_started_total",
			metric.WithDescription("OGP cache warms that claimed a warm-pool slot and were detached"))
		cacheWarmShedCounter, _ = meter.Int64Counter("alt_backend_image_cache_warm_shed_total",
			metric.WithDescription("OGP cache warms dropped because the warm pool was already full; a sustained rate means warms arrive faster than the per-host limiter lets them finish"))
	})
}

// logCacheWarmShed records one dropped warm and emits the throttled warning
// that stands for the window's worth of them.
//
// The per-URL detail stays at debug level for the lines the throttle swallows,
// so turning the level down still answers "which images went uncached".
func (h *Handler) logCacheWarmShed(ctx context.Context, ogURL string) {
	cacheWarmShedCounter.Add(ctx, 1)

	h.warmShedMu.Lock()
	h.warmShedCount++
	count := h.warmShedCount
	now := time.Now()
	shouldLog := h.lastWarmShedLog.IsZero() || now.Sub(h.lastWarmShedLog) >= warmShedLogInterval
	if shouldLog {
		h.lastWarmShedLog = now
	}
	h.warmShedMu.Unlock()

	if !shouldLog {
		h.logger.DebugContext(ctx, "cache warm shed, warm pool saturated",
			"url", ogURL, "operation", "BatchPrefetchImages")
		return
	}

	h.logger.WarnContext(ctx, "image_cache_warm.shed",
		"url", ogURL,
		"occurrences", count,
		"pool_size", maxConcurrentCacheWarms,
		"operation", "BatchPrefetchImages",
		"impact", "each dropped warm leaves one image uncached on the next view")
}

// BatchPrefetchImages generates proxy URLs and optionally warms cache for OGP images.
func (h *Handler) BatchPrefetchImages(
	ctx context.Context,
	req *connect.Request[articlesv2.BatchPrefetchImagesRequest],
) (*connect.Response[articlesv2.BatchPrefetchImagesResponse], error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}

	articleIDs := req.Msg.ArticleIds
	if len(articleIDs) == 0 {
		return connect.NewResponse(&articlesv2.BatchPrefetchImagesResponse{}), nil
	}
	if len(articleIDs) > 10 {
		articleIDs = articleIDs[:10]
	}

	if h.deps.ImageProxy == nil {
		return connect.NewResponse(&articlesv2.BatchPrefetchImagesResponse{}), nil
	}

	// Fetch OGP URLs from article_heads, through alt-data-hub.
	ogURLs, err := h.deps.OgImageURLs.FetchOgImageURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "BatchPrefetchImages")
	}

	// Generate proxy URLs
	proxyURLs := h.deps.ImageProxy.BatchGenerateProxyURLs(ctx, ogURLs)

	// Build response
	images := make([]*articlesv2.ImageProxyInfo, 0, len(proxyURLs))
	for articleID, proxyURL := range proxyURLs {
		images = append(images, &articlesv2.ImageProxyInfo{
			ArticleId: articleID,
			ProxyUrl:  proxyURL,
			IsCached:  false, // We don't check cache status in batch for performance
		})
	}

	// Warm cache for images in background. WithoutCancel keeps request values
	// (trace/auth metadata) while allowing work to finish after the RPC returns.
	//
	// Bounded by warmSlots and shed — not queued — when the pool is full: a
	// dropped warm costs one uncached image on the next view, while a queued
	// one costs a goroutine holding a request context for up to a minute, and
	// the queue would only ever grow because warms arrive faster than the
	// per-host limiter lets them finish.
	initCacheWarmMetrics()
	for _, ogURL := range ogURLs {
		select {
		case h.warmSlots <- struct{}{}:
			cacheWarmStartedCounter.Add(ctx, 1)
		default:
			h.logCacheWarmShed(ctx, ogURL)
			continue
		}

		ogURLCopy := ogURL
		go func() {
			defer func() { <-h.warmSlots }()
			warmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
			defer cancel()
			h.deps.ImageProxy.WarmCache(warmCtx, ogURLCopy)
		}()
	}

	return connect.NewResponse(&articlesv2.BatchPrefetchImagesResponse{
		Images: images,
	}), nil
}

// These counters exist for the same reason the image-warm ones do: every
// outcome below the accepted count happens after the response is written, so
// without them a saturated pool, a busy publisher and a batch that was already
// cached are one indistinguishable silence.
//
// Read them as a funnel — accepted, then minus cached, minus host-busy, minus
// failed, equals warms that actually stored a body. A high host-busy share is
// the honest signal that the batch is not spanning enough distinct hosts for
// this feature to be doing anything.
var (
	contentWarmMeterOnce              sync.Once
	contentWarmStartedCounter         metric.Int64Counter
	contentWarmShedCounter            metric.Int64Counter
	contentWarmSkippedCachedCounter   metric.Int64Counter
	contentWarmHostBusyCounter        metric.Int64Counter
	contentWarmProbeFailedCounter     metric.Int64Counter
	contentWarmFetchFailedCounter     metric.Int64Counter
	contentWarmCompletedCounter       metric.Int64Counter
	contentWarmRejectedURLCounter     metric.Int64Counter
	contentWarmSkippedSameHostCounter metric.Int64Counter
)

func initContentWarmMetrics() {
	contentWarmMeterOnce.Do(func() {
		meter := otel.Meter("alt-backend.article-content-warm")
		contentWarmStartedCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_started_total",
			metric.WithDescription("article-body warms that claimed a pool slot and were detached"))
		contentWarmShedCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_shed_total",
			metric.WithDescription("article-body warms dropped because the warm pool was full; nothing was claimed and no publisher was contacted"))
		contentWarmSkippedCachedCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_skipped_cached_total",
			metric.WithDescription("article-body warms that ended at the store probe because the body was already there"))
		contentWarmHostBusyCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_host_busy_total",
			metric.WithDescription("article-body warms abandoned because the host's turn was taken; the crawl-delay gate was never asked"))
		contentWarmProbeFailedCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_probe_failed_total",
			metric.WithDescription("article-body warms abandoned because the store probe could not answer"))
		contentWarmFetchFailedCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_fetch_failed_total",
			metric.WithDescription("article-body warms that reached the publisher and did not come back with a body"))
		contentWarmCompletedCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_completed_total",
			metric.WithDescription("article-body warms that stored a body"))
		contentWarmRejectedURLCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_rejected_url_total",
			metric.WithDescription("prefetch URLs refused before anything was claimed: unparseable or outside the SSRF allowlist"))
		contentWarmSkippedSameHostCounter, _ = meter.Int64Counter("alt_backend_article_content_warm_skipped_same_host_total",
			metric.WithDescription("prefetch URLs dropped because an earlier entry in the same batch already claimed that host"))
	})
}

// BatchPrefetchArticleContent warms the article bodies the reader believes the
// user is about to open, and returns before any of them is fetched.
//
// The shape is BatchPrefetchImages': a capped list, a fixed pool of detached
// warms, shed rather than queued when the pool is full, and a
// context.WithoutCancel with its own budget so a warm outlives the RPC that
// asked for it. What differs is what a warm costs. An OGP warm reads a CDN on
// a one-second interval; an article warm crawls a publisher on a ten-second
// one, and on the way it passes a gate that *reserves* that publisher's
// crawl-delay window simply by being asked. So the order below is not
// incidental:
//
//	validate → one per host → claim a pool slot   (synchronous, cheap, free to lose)
//	  → probe the store                            (free)
//	    → take the host's turn                     (authoritative; refusal costs nothing)
//	      → ask the policy gate and fetch          (reserves the window — last, and once)
//
// Every step that can drop the work is placed above the step that cannot be
// undone. The reverse order — ask the gate, then discover the pool or the host
// is busy — burns the publisher's window on a fetch that never happens, and
// the next thing denied by that window is the user's own read of the article
// they just opened. Background work starving the foreground is the failure
// this ordering exists to prevent.
//
// Errors: this RPC returns exactly two, and neither carries
// X-Alt-Failure-Scope. Unauthenticated and FailedPrecondition are both ours —
// a missing session and a disabled capability. Publisher outcomes are not
// reachable from here at all, because they happen after the response; ADR-000963
// reserves the host scope for failures positively attributed to a publisher,
// and there is no such failure on this path to attribute.
func (h *Handler) BatchPrefetchArticleContent(
	ctx context.Context,
	req *connect.Request[articlesv2.BatchPrefetchArticleContentRequest],
) (*connect.Response[articlesv2.BatchPrefetchArticleContentResponse], error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	// ADR-000966: a capability that is off says so by name, before argument
	// validation, so the caller learns the thing that will not succeed on any
	// retry rather than the thing that happened to be checked first.
	if !h.deps.PrefetchWiring.Enabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("article content prefetch is disabled: %s", h.deps.PrefetchWiring.disabledReason()))
	}

	urls := req.Msg.GetUrls()
	if len(urls) == 0 {
		return connect.NewResponse(&articlesv2.BatchPrefetchArticleContentResponse{}), nil
	}

	initContentWarmMetrics()

	validTargets, rejections, skippedSameHost := dedupeValidPrefetchTargets(urls, maxPrefetchArticleURLs, security.NewURLSecurityValidator().ValidateParsedRSSURL)

	for _, rej := range rejections {
		contentWarmRejectedURLCounter.Add(ctx, 1)
		h.logger.DebugContext(ctx, "article_content_warm.rejected",
			"url", rej.RawURL, "reason", rej.Reason, "error", rej.Err)
	}
	for i := 0; i < skippedSameHost; i++ {
		contentWarmSkippedSameHostCounter.Add(ctx, 1)
	}

	var accepted, shed int32
	for _, target := range validTargets {
		select {
		case h.contentWarmSlots <- struct{}{}:
			contentWarmStartedCounter.Add(ctx, 1)
		default:
			shed++
			h.logContentWarmShed(ctx, target.String())
			continue
		}

		accepted++
		warmURL := target
		warmUser := *user
		go func() {
			defer func() { <-h.contentWarmSlots }()
			// WithoutCancel keeps the request's trace and auth values while
			// letting the warm outlive the RPC; the explicit budget is what
			// keeps "outlives" from meaning "forever".
			warmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), contentWarmBudget)
			defer cancel()
			h.warmArticleContent(warmCtx, warmURL, warmUser)
		}()
	}

	return connect.NewResponse(&articlesv2.BatchPrefetchArticleContentResponse{
		AcceptedCount:        accepted,
		ShedCount:            shed,
		RejectedCount:        int32(len(rejections)),
		SkippedSameHostCount: int32(skippedSameHost),
	}), nil
}

// warmArticleContent runs one detached warm. It returns on the first reason
// not to continue and never retries: this is one of four hops between the
// browser and the publisher, and a retry here multiplies against the others
// rather than adding to them.
func (h *Handler) warmArticleContent(ctx context.Context, target *url.URL, user domain.UserContext) {
	urlStr := target.String()

	// 1. Is it already there? Free to ask, and it is the answer that most
	//    often makes the rest unnecessary.
	stored, probeErr := h.deps.PrefetchProbe.FetchArticleByURL(ctx, urlStr)
	if probeErr != nil {
		// A probe that cannot answer is not a reason to fetch harder. Nobody
		// is waiting on this warm, so the cheap and polite move is to stop.
		contentWarmProbeFailedCounter.Add(ctx, 1)
		h.logger.DebugContext(ctx, "article_content_warm.probe_failed",
			"url", urlStr, "error", probeErr)
		return
	}
	if stored != nil && len(strings.TrimSpace(stored.Content)) >= minPrefetchStoredContentLength {
		contentWarmSkippedCachedCounter.Add(ctx, 1)
		h.logger.DebugContext(ctx, "article_content_warm.already_stored", "url", urlStr)
		return
	}

	// 2. The host's turn, bounded by the prefetch class's budget. Losing this
	//    wait costs nothing — x/time/rate returns the reservation it made and
	//    a lost SET NX holds no slot — which is exactly why it comes before
	//    the gate that cannot give anything back.
	slotCtx, cancel := context.WithTimeout(ctx, h.deps.PrefetchWiring.SlotWait)
	defer cancel()
	if slotErr := h.deps.PrefetchHostSlots.WaitForHost(slotCtx, urlStr); slotErr != nil {
		contentWarmHostBusyCounter.Add(ctx, 1)
		h.logger.DebugContext(ctx, "article_content_warm.host_busy",
			"url", urlStr,
			"budget", h.deps.PrefetchWiring.SlotWait,
			"impact", "warm abandoned; the crawl-delay gate was never asked, so the publisher's window is untouched")
		return
	}

	// 3. The usecase asks the scraping-policy gate and, if it is granted,
	//    issues the request immediately. The turn is already ours, so a grant
	//    here is followed by a real fetch rather than by a shed.
	if _, _, _, fetchErr := h.deps.PrefetchArticle.FetchCompliantArticle(ctx, target, user); fetchErr != nil {
		contentWarmFetchFailedCounter.Add(ctx, 1)
		// Debug, not warn: a publisher that refuses a warm is not an incident,
		// and the reader's own fetch of the same article will report it
		// properly if it happens again there.
		h.logger.DebugContext(ctx, "article_content_warm.fetch_failed",
			"url", urlStr, "error", fetchErr)
		return
	}

	contentWarmCompletedCounter.Add(ctx, 1)
}

// logContentWarmShed records one dropped article warm and emits the throttled
// warning that stands for the window's worth of them. Same shape, and same
// reasoning, as logCacheWarmShed.
func (h *Handler) logContentWarmShed(ctx context.Context, articleURL string) {
	contentWarmShedCounter.Add(ctx, 1)

	h.contentWarmShedMu.Lock()
	h.contentWarmShedCount++
	count := h.contentWarmShedCount
	now := time.Now()
	shouldLog := h.lastContentWarmShedLog.IsZero() || now.Sub(h.lastContentWarmShedLog) >= warmShedLogInterval
	if shouldLog {
		h.lastContentWarmShedLog = now
	}
	h.contentWarmShedMu.Unlock()

	if !shouldLog {
		h.logger.DebugContext(ctx, "article content warm shed, warm pool saturated",
			"url", articleURL, "operation", "BatchPrefetchArticleContent")
		return
	}

	h.logger.WarnContext(ctx, "article_content_warm.shed",
		"url", articleURL,
		"occurrences", count,
		"pool_size", maxConcurrentContentWarms,
		"operation", "BatchPrefetchArticleContent",
		"impact", "each dropped warm leaves one article body to be fetched live when the user opens it")
}
