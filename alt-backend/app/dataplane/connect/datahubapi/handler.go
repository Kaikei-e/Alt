// Package datahubapi implements services.datahub.v1.DataHubService, the only name
// alt-data-hub answers to for the data plane (ADR-000954 D7).
//
// The procedures here were served under two names for the length of Wave 2:
// services.backend.v1.BackendInternalService, from when this code lived inside
// alt-backend, and services.datahub.v1.DataHubService. Wave 2-B moved the five
// consumers one PR at a time; Wave 2-C deleted the old proto, so this package
// now implements the DataHubService messages directly instead of re-encoding
// each call onto a legacy twin. The adapter that did the re-encoding, and the
// descriptor walk that proved the two message trees were the same wire schema,
// went with it — there is no second schema left to diverge from.
//
// Two of the 26 procedures never had an RPC counterpart. GetSystemUser and
// ListRecentArticles are the /v1/internal REST routes that ADR-000954 D6 folds
// into the Connect surface; they call the same usecases the REST handlers did,
// and the REST routes are gone.
package datahubapi

import (
	"context"
	"log/slog"

	"alt/dataplane/port/datahub_capability_port"
	"alt/dataplane/port/internal_article_port"
	"alt/dataplane/port/internal_feed_port"
	"alt/dataplane/port/internal_tag_port"
	"alt/dataplane/usecase/create_tag_set_version_usecase"
	"alt/dataplane/usecase/outbox_usecase"
	"alt/dataplane/usecase/push_delivery_usecase"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
	"alt/shared/port/event_publisher_port"
	"alt/shared/port/knowledge_event_port"
	"alt/shared/usecase/create_summary_version_usecase"
)

const maxLimit = 500

// Defaults copied from the REST routes ADR-000954 D6 absorbs. They live here
// rather than being left to the usecase because the RPC has to distinguish
// "caller omitted the field" from "caller sent zero", and only the delivery
// layer can see that difference.
const (
	defaultRecentWithinHours = 24
	defaultRecentLimit       = 100
)

// SystemUserPort resolves the system identity that service-to-service callers
// act as. Backed by the Kratos client; declared here so the handler depends on
// the one method it uses.
type SystemUserPort interface {
	GetFirstIdentityID(ctx context.Context) (string, error)
}

// RecentArticlesUsecase is the read behind the former GET
// /v1/internal/articles/recent.
type RecentArticlesUsecase interface {
	Execute(ctx context.Context, in fetch_recent_articles_usecase.FetchRecentArticlesInput) (*fetch_recent_articles_usecase.FetchRecentArticlesOutput, error)
}

// Handler implements DataHubServiceHandler.
type Handler struct {
	// Search indexer ports
	listArticles        internal_article_port.ListArticlesWithTagsPort
	listArticlesForward internal_article_port.ListArticlesWithTagsForwardPort
	listDeleted         internal_article_port.ListDeletedArticlesPort
	getLatestTimestamp  internal_article_port.GetLatestArticleTimestampPort
	getArticleByID      internal_article_port.GetArticleByIDPort

	// Pre-processor article ingestion ports
	checkArticleExists internal_article_port.CheckArticleExistsPort
	createArticle      internal_article_port.CreateArticlePort
	saveArticleSummary internal_article_port.SaveArticleSummaryPort
	getArticleContent  internal_article_port.GetArticleContentPort
	getFeedID          internal_feed_port.GetFeedIDPort
	listFeedURLs       internal_feed_port.ListFeedURLsPort

	// Tag-generator ports
	upsertArticleTags        internal_tag_port.UpsertArticleTagsPort
	batchUpsertArticleTags   internal_tag_port.BatchUpsertArticleTagsPort
	listUntaggedArticles     internal_tag_port.ListUntaggedArticlesPort
	batchGetTagsByArticleIDs internal_tag_port.BatchGetTagsByArticleIDsPort

	// Quality checker ports
	deleteArticleSummary      internal_article_port.DeleteArticleSummaryPort
	checkArticleSummaryExists internal_article_port.CheckArticleSummaryExistsPort
	findArticlesWithSummaries internal_article_port.FindArticlesWithSummariesPort

	// Summarization (pre-processor polling)
	listUnsummarized internal_article_port.ListUnsummarizedArticlesPort
	hasUnsummarized  internal_article_port.HasUnsummarizedArticlesPort

	// Backfill (pre-processor split-DB)
	getEmptyFeedID internal_feed_port.GetEmptyFeedIDPort

	// RAG Tool Operations (ADR-000617)
	fetchTagCloudPort      fetchTagCloudPort
	fetchArticlesByTagPort fetchArticlesByTagPort

	// Recap article window (recap-worker paginated fetch).
	recapArticlesUsecase recapArticlesUsecase
	feedsInWindowUsecase feedsInWindowUsecase

	// Absorbed REST routes (ADR-000954 D6). Required — see NewHandler.
	systemUser     SystemUserPort
	recentArticles RecentArticlesUsecase

	// Event publishing. The mq-hub publisher is switchable — it carries
	// notifications, and it says so itself through IsEnabled — but switchable
	// is not the same as omissible: "off" is a configuration the publisher
	// reports, and a nil one is a composition root that forgot the option, so
	// CreateArticle panics on it rather than skipping it.
	eventPublisher event_publisher_port.EventPublisherPort

	// The knowledge event sink is not: it is where CreateArticle appends the
	// ArticleCreated that gives a Knowledge Home row its title and url, so
	// WithKnowledgeEventPort panics on nil and CreateArticle panics if the
	// option was never passed.
	knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort

	// Knowledge version usecases
	createSummaryVersionUsecase *create_summary_version_usecase.CreateSummaryVersionUsecase
	createTagSetVersionUsecase  *create_tag_set_version_usecase.CreateTagSetVersionUsecase

	// Media and cache capabilities (ADR-000954 D3, catalog §2.A / §2.D / §2.E / §2.L /
	// §2.O): the capabilities alt-backend and alt-harvester used to reach by
	// opening their own alt_db pool.
	//
	// Unlike the optional ports above, these are required rather than
	// optional — see WithMediaCapabilities. They are not a feature of the
	// data plane, they are the data plane: the two callers have no other route
	// to the outbox, the image cache or the scraping policy once their pools
	// are gone, and a handler serving Unimplemented for them would look
	// exactly like a retired procedure.
	outboxUsecase   *outbox_usecase.OutboxUsecase
	ogImage         datahub_capability_port.OgImagePort
	imageProxyCache datahub_capability_port.ImageProxyCachePort
	scrapingPolicy  datahub_capability_port.ScrapingPolicyPort
	autoFulltext    datahub_capability_port.AutoFulltextPort

	// Article capabilities (catalog §2.B / §2.C / §2.N). Same rule: required, and
	// WithArticleCapabilities panics on nil rather than letting the
	// article surface answer Unimplemented.
	articleWrite      datahub_capability_port.ArticleWritePort
	articleRead       datahub_capability_port.ArticleReadPort
	knowledgeBackfill datahub_capability_port.KnowledgeBackfillPort

	// Feed and feed-link capabilities (catalog §2.F / §2.G / §2.H). Same rule again: required,
	// and WithFeedCapabilities panics on nil. These are the feed list,
	// the subscription list and the collector's poll-health state machine —
	// the surfaces a user notices first when they answer nothing.
	feedLink             datahub_capability_port.FeedLinkPort
	feedLinkAvailability datahub_capability_port.FeedLinkAvailabilityPort
	feed                 datahub_capability_port.FeedPort

	// Read-state and tag-read capabilities (catalog §2.I / §2.J). Required, and
	// WithReadStateAndTagCapabilities panics on nil. These carry the per-user
	// state — read marks, subscriptions, favourites — and every tag surface;
	// a nil one makes a user's actions vanish rather than fail.
	readState datahub_capability_port.ReadStatePort
	tagRead   datahub_capability_port.TagReadPort

	// Versioned artifact and stats capabilities (catalog §2.K / §2.M). Required, and
	// WithVersionAndStatsCapabilities panics on nil. The version ports carry the
	// append-first invariant — including the two advisory-locked supersedes,
	// the only place in this service where correctness depends on a lock
	// rather than on a constraint — and the stats port carries every dashboard
	// number.
	summaryVersion datahub_capability_port.SummaryVersionPort
	tagSetVersion  datahub_capability_port.TagSetVersionPort
	stats          datahub_capability_port.StatsPort

	// Tag Trail and article reference capabilities (catalog §2.J / §2.C) — the last two, after which
	// alt-backend has no database pool at all. Required, and
	// WithTagTrailCapabilities panics on nil: neither of these answering
	// nothing looks like an error to its caller, so an unwired one produces a
	// working-looking product with two features quietly missing.
	tagTrail   datahub_capability_port.TagTrailPort
	articleRef datahub_capability_port.ArticleRefPort

	// Web Push storage. Required, and WithPushCapabilities panics on nil: the
	// subscription table is the only route alt.push.v1.PushService has, and an
	// unwired delivery queue answers a dispatcher's claim exactly the way a
	// drained one does.
	//
	// The delivery queue gets a usecase and the subscription table does not,
	// for the reason the outbox did: there is a state machine here spread
	// across several driver calls, and there is none over there.
	pushSubscriptions datahub_capability_port.PushSubscriptionPort
	pushDeliveries    *push_delivery_usecase.PushDeliveryUsecase

	logger *slog.Logger
}

var _ datahubv1connect.DataHubServiceHandler = (*Handler)(nil)

// NewHandler creates a new DataHubService handler.
//
// systemUser and recentArticles are positional and required rather than
// functional options, and a missing one panics. They are not features that can
// be switched off: they are the only implementation of two procedures the
// service advertises, and a handler built without them would answer the first
// real call with a nil dereference. Returning Unimplemented instead would be
// worse — that is also what a genuinely retired procedure answers, and
// CLAUDE.md rule 8 exists because those two states became indistinguishable
// once before (ADR-000928). Refusing at construction makes a DI mistake a
// process that does not start.
//
// Note for callers holding concrete types: a nil *T stored in one of these
// interfaces is not nil as an interface value and will not be caught here. The
// composition root checks its own fields before calling — see
// connect/v2/datahub/server.go.
func NewHandler(
	listArticles internal_article_port.ListArticlesWithTagsPort,
	listArticlesForward internal_article_port.ListArticlesWithTagsForwardPort,
	listDeleted internal_article_port.ListDeletedArticlesPort,
	getLatestTimestamp internal_article_port.GetLatestArticleTimestampPort,
	getArticleByID internal_article_port.GetArticleByIDPort,
	systemUser SystemUserPort,
	recentArticles RecentArticlesUsecase,
	logger *slog.Logger,
	opts ...HandlerOption,
) *Handler {
	switch {
	case systemUser == nil:
		panic("datahubapi: SystemUserPort is required — " +
			"DataHubService.GetSystemUser replaces GET /v1/internal/system-user")
	case recentArticles == nil:
		panic("datahubapi: RecentArticlesUsecase is required — " +
			"DataHubService.ListRecentArticles replaces GET /v1/internal/articles/recent")
	}
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{
		listArticles:        listArticles,
		listArticlesForward: listArticlesForward,
		listDeleted:         listDeleted,
		getLatestTimestamp:  getLatestTimestamp,
		getArticleByID:      getArticleByID,
		systemUser:          systemUser,
		recentArticles:      recentArticles,
		logger:              logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func clampLimit(limit int) int {
	if limit <= 0 {
		limit = 200
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit
}
