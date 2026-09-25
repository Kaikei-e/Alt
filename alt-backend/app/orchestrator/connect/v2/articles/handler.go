// Package articles implements the ArticleService Connect-RPC handlers.
package articles

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"alt/config"
	"alt/domain"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"
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
)

// maxArticlesPageSize is the largest page the cursor RPCs will serve.
//
// It sits one below the usecases' own ceiling of 100 on purpose: these
// handlers ask for limit+1 rows so they can answer has_more without a
// separate COUNT, so a page of 100 would fetch 101 and the usecase would
// reject it. Clamping to 100 turned the documented maximum page size into
// an opaque CodeInternal.
const maxArticlesPageSize = 99

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
