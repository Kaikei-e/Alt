// Package articles implements the ArticleService Connect-RPC handlers.
package articles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"

	"alt/config"
	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	"alt/orchestrator/usecase/archive_article_usecase"
	"alt/orchestrator/usecase/fetch_article_summary_usecase"
	"alt/orchestrator/usecase/fetch_article_tags_usecase"
	"alt/orchestrator/usecase/fetch_article_usecase"
	"alt/orchestrator/usecase/fetch_articles_usecase"
	"alt/orchestrator/usecase/fetch_inoreader_summary_usecase"
	"alt/orchestrator/usecase/fetch_latest_article_usecase"
	"alt/orchestrator/usecase/fetch_random_subscription_usecase"
	"alt/orchestrator/usecase/get_article_source_url_usecase"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/orchestrator/usecase/stream_article_tags_usecase"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/shared/usecase/fetch_tag_cloud_usecase"
	"alt/utils/perf"
	"alt/utils/safeconv"
	"alt/utils/url_validator"

	"google.golang.org/protobuf/proto"
)

// OgImageURLLookup resolves og:image URLs for a batch of articles.
//
// Declared as a port here because BatchPrefetchImages used to call
// AltDBRepository.FetchOgImageURLsByArticleIDs directly — a handler reaching
// past the usecase and gateway layers into the database driver. ADR-000954
// Wave 3 moved the query to alt-data-hub (catalog §2.D / W3-D2), which made
// the shortcut impossible rather than merely discouraged: there is no pool in
// this process to reach through any more.
type OgImageURLLookup interface {
	FetchOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error)
}

// HostSlotGate is this process's turn-taking gate for one third-party host —
// the shared *rate_limiter.HostRateLimiter, under NamespaceExternalAPI.
//
// BatchPrefetchArticleContent holds it directly instead of leaving it to the
// fetch gateway, because *when* the turn is taken decides whether a prefetch
// can hurt the reader. Losing this wait consumes nothing (the in-process
// bucket restores its reservation and a lost SET NX takes no slot), while the
// scraping-policy gate further down reserves the publisher's crawl-delay
// window the moment it grants. Taking the free-to-lose gate first is what
// keeps a prefetch from spending a window it will not use.
//
// It is deliberately the *same* limiter and the same namespace the interactive
// fetch uses. A second namespace would let prefetch and read traffic each get
// a turn per interval, doubling what the publisher actually sees while every
// configured number stayed the same.
type HostSlotGate interface {
	WaitForHost(ctx context.Context, rawURL string) error
}

// StoredArticleProbe answers "is this body already in the store?" without
// contacting anyone. It is the first thing a warm asks: an article already
// stored has nothing to warm, and asking the host for a turn to discover that
// would spend an interval the reader's next real fetch needs.
type StoredArticleProbe interface {
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
}

// ArticlePrefetchWiring declares whether this binary can warm article bodies.
//
// It is a declaration, not an inference from a nil dependency (ADR-000966 §2):
// the ports behind a prefetch are all shared with the interactive read path,
// so their presence says nothing about whether warming is meant to happen.
// Exactly one of the two states is logged by the composition root at startup,
// and the disabled one is reachable only from an explicit config value.
type ArticlePrefetchWiring struct {
	// Enabled declares that this binary warms article bodies on request.
	Enabled bool

	// DisabledReason is handed to the caller verbatim in the
	// FAILED_PRECONDITION message. It names the setting to change, so an
	// operator learns what is off rather than that "something" is.
	DisabledReason string

	// SlotWait bounds how long a warm queues for its turn at a host before
	// giving the turn up. This is the third fetch class: a background job
	// waits as long as its context allows, an interactive fetch waits
	// RATE_LIMIT_INTERACTIVE_SLOT_WAIT because a user is watching, and a
	// prefetch has nobody watching at all — so it takes only a turn that is
	// already free and abandons the rest. Zero is refused at construction:
	// for this class it would mean "queue like a background job", which is the
	// priority inversion the class exists to avoid.
	SlotWait time.Duration
}

func (w ArticlePrefetchWiring) disabledReason() string {
	if w.DisabledReason != "" {
		return w.DisabledReason
	}
	return "no article prefetch wiring was declared by the composition root"
}

// ArticleHandlerDeps holds the dependencies for the Article service handler.
type ArticleHandlerDeps struct {
	OgImageURLs             OgImageURLLookup
	ArchiveArticle          *archive_article_usecase.ArchiveArticleUsecase
	Article                 fetch_article_usecase.ArticleUsecase
	FetchArticlesByTag      *fetch_articles_by_tag_usecase.FetchArticlesByTagUsecase
	FetchArticlesCursor     *fetch_articles_usecase.FetchArticlesCursorUsecase
	FetchArticleSummary     *fetch_article_summary_usecase.FetchArticleSummaryUsecase
	FetchArticleTags        *fetch_article_tags_usecase.FetchArticleTagsUsecase
	FetchInoreaderSummary   fetch_inoreader_summary_usecase.FetchInoreaderSummaryUsecase
	FetchLatestArticle      *fetch_latest_article_usecase.FetchLatestArticleUsecase
	FetchRandomSubscription *fetch_random_subscription_usecase.FetchRandomSubscriptionUsecase
	FetchTagCloud           *fetch_tag_cloud_usecase.FetchTagCloudUsecase
	GetArticleSourceURL     *get_article_source_url_usecase.GetArticleSourceURLUsecase
	ImageProxy              *image_proxy_usecase.ImageProxyUsecase
	StreamArticleTags       *stream_article_tags_usecase.StreamArticleTagsUsecase

	// The article-content prefetch trio. PrefetchArticle is a *second*
	// ArticleUsecase over the same repository, robots and scraping-policy
	// ports as Article, differing only in its fetch gateway — see
	// di/article_module.go. Sharing the scraping-policy instance is load
	// bearing: crawl-delay state lives in that gateway, so a second instance
	// would hand the reader and the warmer a turn each inside one delay.
	PrefetchArticle   fetch_article_usecase.ArticleUsecase
	PrefetchHostSlots HostSlotGate
	PrefetchProbe     StoredArticleProbe
	PrefetchWiring    ArticlePrefetchWiring
}

// Handler implements the ArticleService Connect-RPC service.
type Handler struct {
	deps   ArticleHandlerDeps
	logger *slog.Logger
	cfg    *config.Config

	// warmSlots is the fixed pool of detached cache warms BatchPrefetchImages
	// is allowed to have in flight. Sending claims a slot, the goroutine
	// returns it. See maxConcurrentCacheWarms.
	warmSlots chan struct{}

	// Throttle state for the shed warning. See logCacheWarmShed.
	warmShedMu      sync.Mutex
	warmShedCount   int
	lastWarmShedLog time.Time

	// contentWarmSlots is the article-body equivalent of warmSlots, kept
	// separate on purpose: an OGP warm is a CDN read on a 1s interval, an
	// article warm is a publisher crawl on a 10s one, and one pool would let
	// the cheap traffic decide how much of the expensive traffic runs.
	contentWarmSlots chan struct{}

	contentWarmShedMu      sync.Mutex
	contentWarmShedCount   int
	lastContentWarmShedLog time.Time
}

// NewHandler creates a new Article service handler.
//
// It panics when the prefetch capability is declared enabled but not actually
// wired. That is a composition-root bug, not an operator setting, and the
// alternative — discovering it as a nil check inside the RPC — is precisely
// what CLAUDE.md rule 8 forbids: it makes "DI forgot" indistinguishable from
// "deliberately off". Panicking here rather than in the request path is what
// keeps ADR-000966's objection (an operator must not be able to crash the
// service by pressing a button) from applying: nobody is waiting on a process
// that has not finished starting.
func NewHandler(deps ArticleHandlerDeps, cfg *config.Config, logger *slog.Logger) *Handler {
	if deps.PrefetchWiring.Enabled {
		switch {
		case deps.PrefetchArticle == nil:
			panic("article prefetch declared enabled but PrefetchArticle usecase is nil")
		case deps.PrefetchHostSlots == nil:
			panic("article prefetch declared enabled but PrefetchHostSlots is nil")
		case deps.PrefetchProbe == nil:
			panic("article prefetch declared enabled but PrefetchProbe is nil")
		case deps.PrefetchWiring.SlotWait <= 0:
			panic("article prefetch declared enabled with a non-positive SlotWait: " +
				"zero would queue a warm behind a user who is waiting")
		}
	}

	return &Handler{
		deps:             deps,
		logger:           logger,
		cfg:              cfg,
		warmSlots:        make(chan struct{}, maxConcurrentCacheWarms),
		contentWarmSlots: make(chan struct{}, maxConcurrentContentWarms),
	}
}

// Verify interface implementation at compile time.
var _ articlesv2connect.ArticleServiceHandler = (*Handler)(nil)

const (
	// FailureScopeHeader names how far a failure reaches. Connect error
	// metadata is merged into the unary response headers, so it survives the
	// BFF's transparent proxy and reaches connect-es as ConnectError.metadata.
	//
	// It exists because CodeUnavailable is issued by two parties that mean
	// opposite things by it: alt-backend for a single publisher that did not
	// answer, and the BFF for a breaker that is open against every host. A
	// client that cannot tell them apart must guess, and guessing "global"
	// let one dead link black the whole reader out for a full cooldown.
	FailureScopeHeader = "X-Alt-Failure-Scope"

	// FailureScopeHost means the failure belongs to one third-party host.
	// Every other host is still reachable and alt-backend is healthy, so this
	// must not charge a shared failure budget or pause unrelated work.
	//
	// Only stamp it on errors positively attributed to a publisher. Our own
	// politeness gate is not a publisher's health, and an unclassified fault
	// is still ours — excusing either from the breaker would hide a real
	// outage behind "the site is slow".
	FailureScopeHost = "host"

	// maxArticlesPageSize is the largest page the cursor RPCs will serve.
	//
	// It sits one below the usecases' own ceiling of 100 on purpose: these
	// handlers ask for limit+1 rows so they can answer has_more without a
	// separate COUNT, so a page of 100 would fetch 101 and the usecase would
	// reject it. Clamping to 100 turned the documented maximum page size into
	// an opaque CodeInternal.
	maxArticlesPageSize = 99

	// maxConcurrentCacheWarms bounds the detached OGP cache warms a handler
	// may hold at once. BatchPrefetchImages hands each warm a WithoutCancel
	// context and a 60-second budget, so a warm outlives the RPC that asked
	// for it; the Connect listener has no rate limit of its own and WarmCache
	// parks on the per-host limiter, so an unbounded fan-out grows with the
	// arrival rate instead of the completion rate.
	maxConcurrentCacheWarms = 32

	// maxPrefetchArticleURLs bounds one BatchPrefetchArticleContent call.
	//
	// Five, not ten, because the unit that matters is hosts rather than URLs:
	// the per-host interval means a batch can only ever warm one item per host
	// per interval, so a longer list buys nothing and only widens the window
	// in which a warm can be holding a turn the reader wants.
	maxPrefetchArticleURLs = 5

	// maxConcurrentContentWarms bounds the detached article-body warms in
	// flight. Each one may hold a publisher's turn for a full interval and
	// then spend up to the usecase's external-fetch budget on the response, so
	// this is the ceiling on how much of the process's politeness allowance
	// background warming may occupy at once.
	maxConcurrentContentWarms = 8

	// contentWarmBudget is the detached warm's whole life: the host-slot wait,
	// the policy check, the fetch, the extraction and the store write. It is
	// generous relative to the 8s external-fetch timeout inside the usecase
	// because a warm that is cut off *after* the policy gate granted has spent
	// a publisher's crawl-delay window for nothing.
	contentWarmBudget = 30 * time.Second

	// minPrefetchStoredContentLength mirrors the floor
	// fetch_article_usecase applies when deciding whether stored content
	// counts as a hit. It is duplicated rather than exported because the
	// consequence of the two drifting apart is bounded: too low and a warm is
	// skipped that the usecase would have re-fetched, too high and a warm runs
	// that the usecase then answers from the store. Neither is a correctness
	// bug, and the usecase re-checks authoritatively either way.
	minPrefetchStoredContentLength = 100

	// warmShedLogInterval throttles the shed warning. The warning has to be
	// loud — shedding is thrown-away work — but one line per dropped warm is
	// hundreds per second exactly when the pool is saturated and the log is
	// least readable, so it is emitted once per window with the number of
	// occurrences it stands for (same shape as host_rate_limiter's
	// degraded_to_local warning).
	warmShedLogInterval = 30 * time.Second
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

// withFailureScope stamps the blast radius onto a Connect error.
func withFailureScope(err *connect.Error, scope string) *connect.Error {
	err.Meta().Set(FailureScopeHeader, scope)
	return err
}

// FetchArticleContent fetches and extracts compliant article content.
// Replaces GET /v1/articles/fetch/content
func (h *Handler) FetchArticleContent(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleContentRequest],
) (*connect.Response[articlesv2.FetchArticleContentResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate URL
	if req.Msg.Url == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("url is required"))
	}

	parsedURL, err := url.Parse(req.Msg.Url)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid URL format: %w", err))
	}

	// Check for allowed URLs (SSRF protection)
	if err := url_validator.IsAllowedURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("URL not allowed: %w", err))
	}

	// Call usecase
	forceRefresh := req.Msg.ForceRefresh != nil && *req.Msg.ForceRefresh
	content, articleID, ogImageURL, err := h.deps.Article.FetchCompliantArticleWithRefresh(ctx, parsedURL, *user, forceRefresh)
	if err != nil {
		// Checked before ComplianceError: a politeness gate that has not
		// elapsed is transient and must reach the client as a retryable 429,
		// not as the permanent 403 that tells it to stop asking.
		var rateErr *domain.RateLimitedError
		if errors.As(err, &rateErr) {
			connectErr := connect.NewError(connect.CodeResourceExhausted,
				fmt.Errorf("%s", rateErr.Message))
			if rateErr.RetryAfter > 0 {
				connectErr.Meta().Set("Retry-After",
					strconv.Itoa(int(math.Ceil(rateErr.RetryAfter.Seconds()))))
			}
			return nil, connectErr
		}

		var complianceErr *domain.ComplianceError
		if errors.As(err, &complianceErr) {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("%s", complianceErr.Message))
		}

		// The publisher answered, just not with what we wanted. Whatever the
		// status, the verdict is about that one site — hence FailureScopeHost
		// on every branch, including the CodeUnavailable default that is
		// otherwise indistinguishable from the BFF's own breaker rejection.
		var httpErr *domain.ExternalHTTPError
		if errors.As(err, &httpErr) {
			switch {
			case httpErr.StatusCode == 404 || httpErr.StatusCode == 410:
				return nil, withFailureScope(connect.NewError(connect.CodeNotFound,
					fmt.Errorf("article not found")), FailureScopeHost)
			case httpErr.StatusCode == 403 || httpErr.StatusCode == 401:
				return nil, withFailureScope(connect.NewError(connect.CodePermissionDenied,
					fmt.Errorf("access denied by external site")), FailureScopeHost)
			case httpErr.StatusCode == 429:
				return nil, withFailureScope(connect.NewError(connect.CodeResourceExhausted,
					fmt.Errorf("rate limited by external site")), FailureScopeHost)
			default:
				return nil, withFailureScope(connect.NewError(connect.CodeUnavailable,
					fmt.Errorf("external site returned %d", httpErr.StatusCode)), FailureScopeHost)
			}
		}

		// The publisher never answered — a slow site, a dead host, or a wait
		// that ran out. alt-backend is healthy, so this is neither an internal
		// fault to the client nor an ERROR line for whoever reads the log.
		var upstreamErr *domain.UpstreamFetchError
		if errors.As(err, &upstreamErr) {
			h.logger.WarnContext(ctx, "upstream site did not complete the fetch",
				"url", upstreamErr.URL,
				"cause", upstreamErr.Cause,
				"operation", "FetchArticleContent")
			return nil, withFailureScope(connect.NewError(connect.CodeUnavailable,
				fmt.Errorf("the source site did not respond; please try again later")),
				FailureScopeHost)
		}

		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticleContent")
	}

	// Content is already sanitized by usecase (ExtractArticleHTML)
	resp := &articlesv2.FetchArticleContentResponse{
		Url:        parsedURL.String(),
		Content:    content,
		ArticleId:  articleID,
		OgImageUrl: ogImageURL,
		// Signed here, with the same signer the feeds handler mints with
		// (see feeds.enrichWithProxyURLs). Without it this RPC's only image
		// field was the publisher's own URL, so every consumer that wanted a
		// thumbnail put a third-party host into an <img src> — an unproxied
		// cross-origin request that skips the rate limiting, SSRF validation,
		// domain allow-list and re-encoding /v1/images/proxy applies.
		//
		// Empty when the image proxy is switched off, which is an explicit
		// operator decision announced at startup by di/image_module.go's
		// image_proxy_disabled log, not an unwired dependency: with no secret
		// there is nothing to sign with. Clients must render no thumbnail
		// rather than fall back to OgImageUrl.
		OgImageProxyUrl: h.signedOgImageURL(ogImageURL),
	}

	return connect.NewResponse(resp), nil
}

// signedOgImageURL returns the HMAC-gated proxy path for an og:image, or "" if
// there is no image or no image proxy configured.
func (h *Handler) signedOgImageURL(ogImageURL string) string {
	if h.deps.ImageProxy == nil || ogImageURL == "" {
		return ""
	}
	return h.deps.ImageProxy.GenerateProxyURL(ogImageURL)
}

// ArchiveArticle archives an article for later reading.
// Replaces POST /v1/articles/archive
func (h *Handler) ArchiveArticle(
	ctx context.Context,
	req *connect.Request[articlesv2.ArchiveArticleRequest],
) (*connect.Response[articlesv2.ArchiveArticleResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate URL
	if req.Msg.FeedUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_url is required"))
	}

	parsedURL, err := url.Parse(req.Msg.FeedUrl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid URL format: %w", err))
	}

	// Check for allowed URLs (SSRF protection)
	if err := url_validator.IsAllowedURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("URL not allowed: %w", err))
	}

	// Prepare input
	input := archive_article_usecase.ArchiveArticleInput{
		URL:   parsedURL.String(),
		Title: "",
	}
	if req.Msg.Title != nil {
		input.Title = *req.Msg.Title
	}

	// Call usecase
	if err := h.deps.ArchiveArticle.Execute(ctx, input); err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "ArchiveArticle")
	}

	return connect.NewResponse(&articlesv2.ArchiveArticleResponse{
		Message: "article archived",
	}), nil
}

// FetchArticlesCursor fetches articles with cursor-based pagination.
// Replaces GET /v1/articles/fetch/cursor
func (h *Handler) FetchArticlesCursor(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticlesCursorRequest],
) (*connect.Response[articlesv2.FetchArticlesCursorResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20 // default
	}
	if limit > maxArticlesPageSize {
		limit = maxArticlesPageSize
	}

	// Parse cursor if provided
	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	// Call usecase (request limit+1 to determine hasMore)
	timer := perf.NewFeedReadTimer("FetchArticlesCursor")

	stopUsecase := timer.StartPhase(ctx, "usecase")
	articles, err := h.deps.FetchArticlesCursor.Execute(ctx, cursor, limit+1)
	stopUsecase()
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticlesCursor")
	}

	// Determine hasMore and trim result
	hasMore := len(articles) > limit
	if hasMore {
		articles = articles[:limit]
	}

	// Count tags across all articles
	tagCount := 0
	for _, a := range articles {
		tagCount += len(a.Tags)
	}

	// Convert to proto
	stopMarshal := timer.StartPhase(ctx, "marshal")
	protoArticles := convertArticlesToProto(articles)

	// Derive next cursor, with sub-second precision: it comes back as the
	// right-hand side of a strict `created_at < $1`, and created_at is
	// microsecond precision.
	var nextCursor *string
	if hasMore && len(articles) > 0 {
		lastArticle := articles[len(articles)-1]
		cursorStr := lastArticle.PublishedAt.Format(time.RFC3339Nano)
		nextCursor = &cursorStr
	}

	respMsg := &articlesv2.FetchArticlesCursorResponse{
		Data:       protoArticles,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
	resp := connect.NewResponse(respMsg)
	stopMarshal()

	timer.SetRowCount(len(articles))
	timer.SetPayloadBytes(int64(proto.Size(respMsg)))
	timer.SetTagCount(tagCount)
	timer.Log(ctx)
	return resp, nil
}

// convertArticlesToProto converts domain articles to proto format.
func convertArticlesToProto(articles []*domain.Article) []*articlesv2.ArticleItem {
	result := make([]*articlesv2.ArticleItem, 0, len(articles))
	for _, article := range articles {
		result = append(result, &articlesv2.ArticleItem{
			Id:          article.ID.String(),
			Title:       article.Title,
			Url:         article.URL,
			Content:     article.Content,
			PublishedAt: article.PublishedAt.Format(time.RFC3339),
			Tags:        article.Tags,
		})
	}
	return result
}

// FetchArticlesByTag fetches articles by tag (ID or name).
// Replaces GET /v1/articles/by-tag
// ADR-169: tag_name で横断検索、tag_id は後方互換性
func (h *Handler) FetchArticlesByTag(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticlesByTagRequest],
) (*connect.Response[articlesv2.FetchArticlesByTagResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20 // default
	}
	if limit > maxArticlesPageSize {
		limit = maxArticlesPageSize
	}

	// Parse cursor if provided
	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	// Request limit+1 to determine hasMore
	var articles []*domain.TagTrailArticle

	// Prefer tag_name (ADR-169 cross-feed discovery), fallback to tag_id
	if req.Msg.TagName != nil && *req.Msg.TagName != "" {
		articles, err = h.deps.FetchArticlesByTag.ExecuteByTagName(ctx, *req.Msg.TagName, cursor, limit+1)
	} else if req.Msg.TagId != nil && *req.Msg.TagId != "" {
		articles, err = h.deps.FetchArticlesByTag.Execute(ctx, *req.Msg.TagId, cursor, limit+1)
	} else {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("either tag_id or tag_name is required"))
	}

	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticlesByTag")
	}

	// Determine hasMore and trim result
	hasMore := len(articles) > limit
	if hasMore {
		articles = articles[:limit]
	}

	// Convert to proto
	protoArticles := make([]*articlesv2.TagTrailArticleItem, 0, len(articles))
	for _, article := range articles {
		protoArticles = append(protoArticles, &articlesv2.TagTrailArticleItem{
			Id:          article.ID,
			Title:       article.Title,
			Link:        article.Link,
			PublishedAt: article.PublishedAt.Format(time.RFC3339),
			FeedTitle:   article.FeedTitle,
		})
	}

	// Derive next cursor, with sub-second precision: it comes back as the
	// right-hand side of a strict `created_at < $1`, and created_at is
	// microsecond precision.
	var nextCursor *string
	if hasMore && len(articles) > 0 {
		lastArticle := articles[len(articles)-1]
		cursorStr := lastArticle.PublishedAt.Format(time.RFC3339Nano)
		nextCursor = &cursorStr
	}

	return connect.NewResponse(&articlesv2.FetchArticlesByTagResponse{
		Articles:   protoArticles,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

// FetchArticleTags fetches tags for an article.
// Replaces GET /v1/articles/:id/tags
func (h *Handler) FetchArticleTags(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleTagsRequest],
) (*connect.Response[articlesv2.FetchArticleTagsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	articleID := req.Msg.ArticleId
	if articleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_id is required"))
	}

	tags, err := h.deps.FetchArticleTags.Execute(ctx, articleID)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticleTags")
	}

	// Convert to proto
	protoTags := make([]*articlesv2.ArticleTagItem, 0, len(tags))
	for _, tag := range tags {
		protoTags = append(protoTags, &articlesv2.ArticleTagItem{
			Id:        tag.ID,
			Name:      tag.TagName,
			CreatedAt: tag.CreatedAt.Format(time.RFC3339),
		})
	}

	return connect.NewResponse(&articlesv2.FetchArticleTagsResponse{
		ArticleId: articleID,
		Tags:      protoTags,
	}), nil
}

// FetchRandomFeed fetches a random feed for Tag Trail.
// Replaces GET /v1/rss-feed-link/random
// ADR-173: Includes tags for the feed's latest article (generated on-the-fly if not in DB)
func (h *Handler) FetchRandomFeed(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchRandomFeedRequest],
) (*connect.Response[articlesv2.FetchRandomFeedResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feed, err := h.deps.FetchRandomSubscription.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchRandomFeed")
	}

	// Fetch tags for the feed's latest article (ADR-173)
	var protoTags []*articlesv2.ArticleTagItem
	var latestArticleID string

	if h.deps.FetchLatestArticle != nil {
		// Get the latest article for this feed
		latestArticle, err := h.deps.FetchLatestArticle.Execute(ctx, feed.ID)
		if err != nil {
			h.logger.WarnContext(ctx, "failed to fetch latest article for feed", "feedID", feed.ID, "error", err)
			// Continue without tags - fail-open
		} else if latestArticle != nil {
			latestArticleID = latestArticle.ID
			h.logger.InfoContext(ctx, "found latest article for feed", "feedID", feed.ID, "articleID", latestArticle.ID)

			// Use FetchArticleTagsUsecase for on-the-fly generation (ADR-168)
			tags, err := h.deps.FetchArticleTags.Execute(ctx, latestArticle.ID)
			if err != nil {
				h.logger.WarnContext(ctx, "failed to fetch/generate tags for article", "articleID", latestArticle.ID, "error", err)
				// Continue without tags - fail-open (frontend can use latestArticleId for streaming)
			} else {
				protoTags = convertTagsToProto(tags)
				h.logger.InfoContext(ctx, "fetched tags for feed's latest article",
					"feedID", feed.ID,
					"articleID", latestArticle.ID,
					"tagCount", len(protoTags))
			}
		} else {
			h.logger.InfoContext(ctx, "no articles found for feed", "feedID", feed.ID)
		}
	}

	return connect.NewResponse(&articlesv2.FetchRandomFeedResponse{
		Id:              feed.ID.String(),
		Url:             feed.WebsiteURL, // Site URL (feeds.website_url, ADR-000868)
		Title:           feed.Title,
		Description:     feed.Description,
		Tags:            protoTags,
		LatestArticleId: latestArticleID,
	}), nil
}

// StreamArticleTags streams real-time tag updates for an article.
// Returns cached tags immediately if available, otherwise triggers on-the-fly generation via mq-hub.
// ADR-168: On-the-fly tag generation for Tag Trail initial feed card.
func (h *Handler) StreamArticleTags(
	ctx context.Context,
	req *connect.Request[articlesv2.StreamArticleTagsRequest],
	stream *connect.ServerStream[articlesv2.StreamArticleTagsResponse],
) error {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	articleID := req.Msg.ArticleId
	if articleID == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_id is required"))
	}

	h.logger.InfoContext(ctx, "starting article tags stream", "articleID", articleID)

	if h.deps.StreamArticleTags == nil {
		h.logger.WarnContext(ctx, "StreamArticleTagsUsecase not available, returning empty tags", "articleID", articleID)
		return stream.Send(&articlesv2.StreamArticleTagsResponse{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("Tag generation not available"),
		})
	}

	result, err := h.deps.StreamArticleTags.Execute(ctx, articleID)
	if err != nil {
		h.logger.WarnContext(ctx, "tag resolution failed", "articleID", articleID, "error", err)
		return stream.Send(&articlesv2.StreamArticleTagsResponse{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("Tag generation temporarily unavailable"),
		})
	}

	if len(result.Tags) == 0 {
		h.logger.InfoContext(ctx, "no tags found or generated", "articleID", articleID)
		return stream.Send(&articlesv2.StreamArticleTagsResponse{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("No tags generated"),
		})
	}

	eventType := articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED
	if result.IsCached {
		eventType = articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED
	}

	h.logger.InfoContext(ctx, "returning tags", "articleID", articleID, "tagCount", len(result.Tags), "cached", result.IsCached)
	return stream.Send(&articlesv2.StreamArticleTagsResponse{
		ArticleId: articleID,
		Tags:      convertTagsToProto(result.Tags),
		EventType: eventType,
	})
}

// convertTagsToProto converts domain tags to proto format.
func convertTagsToProto(tags []*domain.FeedTag) []*articlesv2.ArticleTagItem {
	result := make([]*articlesv2.ArticleTagItem, 0, len(tags))
	for _, tag := range tags {
		result = append(result, &articlesv2.ArticleTagItem{
			Id:        tag.ID,
			Name:      tag.TagName,
			CreatedAt: tag.CreatedAt.Format(time.RFC3339),
		})
	}
	return result
}

// FetchTagCloud fetches tag cloud data for Tag Verse visualization.
func (h *Handler) FetchTagCloud(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchTagCloudRequest],
) (*connect.Response[articlesv2.FetchTagCloudResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 300
	}
	if limit > 500 {
		limit = 500
	}

	items, err := h.deps.FetchTagCloud.Execute(ctx, limit)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchTagCloud")
	}

	protoItems := make([]*articlesv2.TagCloudItem, 0, len(items))
	for _, item := range items {
		protoItems = append(protoItems, &articlesv2.TagCloudItem{
			TagName:      item.TagName,
			ArticleCount: safeconv.Int32(item.ArticleCount),
			PositionX:    float32(item.PositionX),
			PositionY:    float32(item.PositionY),
			PositionZ:    float32(item.PositionZ),
		})
	}

	return connect.NewResponse(&articlesv2.FetchTagCloudResponse{
		Tags:      protoItems,
		TotalTags: safeconv.Int32(len(protoItems)),
	}), nil
}

// stringPtr returns a pointer to a string.
func stringPtr(s string) *string {
	return &s
}

// BatchPrefetchImages generates proxy URLs and optionally warms cache for OGP images.
func (h *Handler) BatchPrefetchImages(
	ctx context.Context,
	req *connect.Request[articlesv2.BatchPrefetchImagesRequest],
) (*connect.Response[articlesv2.BatchPrefetchImagesResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
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

// FetchArticleSummary fetches article summaries for multiple URLs.
// Replaces POST /v1/articles/summary
// Priority: 1) AI-generated summaries from article_summaries table
//
//  2. Inoreader feed excerpts from inoreader_summaries table
func (h *Handler) FetchArticleSummary(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleSummaryRequest],
) (*connect.Response[articlesv2.FetchArticleSummaryResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feedUrls := req.Msg.FeedUrls

	// Validation
	if len(feedUrls) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_urls cannot be empty"))
	}
	if len(feedUrls) > 50 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("maximum 50 URLs allowed"))
	}

	items := make([]*articlesv2.ArticleSummaryItem, 0, len(feedUrls))

	// First, try to get AI-generated summaries from article_summaries table
	if h.deps.FetchArticleSummary != nil {
		for _, feedURL := range feedUrls {
			parsedURL, parseErr := url.Parse(feedURL)
			if parseErr != nil {
				h.logger.WarnContext(ctx, "Failed to parse URL for AI summary lookup",
					"url", feedURL,
					"error", parseErr)
				continue
			}

			// Check for allowed URLs (SSRF protection)
			if ssrfErr := url_validator.IsAllowedURL(parsedURL); ssrfErr != nil {
				h.logger.WarnContext(ctx, "URL not allowed for AI summary lookup",
					"url", feedURL,
					"error", ssrfErr)
				continue
			}

			aiSummary, aiErr := h.deps.FetchArticleSummary.Execute(ctx, parsedURL)
			if aiErr == nil && aiSummary != nil && aiSummary.Summary != "" {
				// Found AI-generated summary
				items = append(items, &articlesv2.ArticleSummaryItem{
					Title:       "AI Summary",
					Content:     aiSummary.Summary,
					Author:      "",
					PublishedAt: time.Now().Format(time.RFC3339),
					FetchedAt:   time.Now().Format(time.RFC3339),
					SourceId:    "",
				})
			}
		}

		// If we found AI summaries, return them
		if len(items) > 0 {
			return connect.NewResponse(&articlesv2.FetchArticleSummaryResponse{
				MatchedArticles: items,
				TotalMatched:    safeconv.Int32(len(items)),
				RequestedCount:  safeconv.Int32(len(feedUrls)),
			}), nil
		}
	}

	// Fallback: Fetch summaries from inoreader_summaries using existing usecase
	summaries, err := h.deps.FetchInoreaderSummary.Execute(ctx, feedUrls)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticleSummary")
	}

	// Convert to proto response
	for _, s := range summaries {
		author := ""
		if s.Author != nil {
			author = *s.Author
		}

		items = append(items, &articlesv2.ArticleSummaryItem{
			Title:       s.Title,
			Content:     s.Content,
			Author:      author,
			PublishedAt: s.PublishedAt.Format(time.RFC3339),
			FetchedAt:   s.FetchedAt.Format(time.RFC3339),
			SourceId:    s.InoreaderID,
		})
	}

	return connect.NewResponse(&articlesv2.FetchArticleSummaryResponse{
		MatchedArticles: items,
		TotalMatched:    safeconv.Int32(len(items)),
		RequestedCount:  safeconv.Int32(len(feedUrls)),
	}), nil
}

// GetArticleSourceURL resolves the canonical external HTTPS URL for an article
// id, scoped to the caller's tenant via JWT. Used by the Knowledge Loop ACT
// workspace's Open recovery affordance when the projection's
// actTargets[].source_url is empty (legacy entry, or producer-side ADR-879
// lookup miss). Read-side query: never appends events.
func (h *Handler) GetArticleSourceURL(
	ctx context.Context,
	req *connect.Request[articlesv2.GetArticleSourceURLRequest],
) (*connect.Response[articlesv2.GetArticleSourceURLResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if h.deps.GetArticleSourceURL == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("get_article_source_url usecase not wired"))
	}

	source, err := h.deps.GetArticleSourceURL.Execute(ctx, req.Msg.GetArticleId(), user.UserID)
	if err != nil {
		switch {
		case errors.Is(err, get_article_source_url_usecase.ErrInvalidArgument):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("malformed article_id"))
		case errors.Is(err, get_article_source_url_usecase.ErrNotFound):
			return nil, connect.NewError(connect.CodeNotFound, errors.New("article not found"))
		default:
			h.logger.ErrorContext(ctx, "get_article_source_url failed", "error", err)
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("source url lookup unavailable"))
		}
	}
	return connect.NewResponse(&articlesv2.GetArticleSourceURLResponse{
		SourceUrl: source.URL,
		Title:     source.Title,
	}), nil
}

// ---------------------------------------------------------------------------
// Article content prefetch
// ---------------------------------------------------------------------------

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
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
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
	if len(urls) > maxPrefetchArticleURLs {
		urls = urls[:maxPrefetchArticleURLs]
	}

	initContentWarmMetrics()

	var accepted, shed, rejected, skippedSameHost int32
	claimedHosts := make(map[string]struct{}, len(urls))

	for _, raw := range urls {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil {
			rejected++
			contentWarmRejectedURLCounter.Add(ctx, 1)
			h.logger.DebugContext(ctx, "article_content_warm.rejected",
				"url", raw, "reason", "unparseable", "error", parseErr)
			continue
		}
		// The same SSRF allowlist FetchArticleContent applies. A batch path
		// that skipped it would be a way to reach the loopback and metadata
		// addresses the single-URL path refuses.
		if allowErr := url_validator.IsAllowedURL(parsed); allowErr != nil {
			rejected++
			contentWarmRejectedURLCounter.Add(ctx, 1)
			h.logger.DebugContext(ctx, "article_content_warm.rejected",
				"url", raw, "reason", "not allowed", "error", allowErr)
			continue
		}

		// One entry per host, decided before anything is claimed. A second
		// URL on a host whose turn this batch already took could only be
		// refused by the crawl-delay gate — after that refusal had cost the
		// gate an ask, and the ask is what reserves the window.
		host := parsed.Host
		if _, taken := claimedHosts[host]; taken {
			skippedSameHost++
			contentWarmSkippedSameHostCounter.Add(ctx, 1)
			continue
		}
		claimedHosts[host] = struct{}{}

		select {
		case h.contentWarmSlots <- struct{}{}:
			contentWarmStartedCounter.Add(ctx, 1)
		default:
			shed++
			h.logContentWarmShed(ctx, parsed.String())
			continue
		}

		accepted++
		warmURL := parsed
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
		RejectedCount:        rejected,
		SkippedSameHostCount: skippedSameHost,
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
