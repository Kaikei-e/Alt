package datahubapi

import (
	"context"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/dataplane/port/internal_article_port"
	"alt/dataplane/port/internal_feed_port"
	"alt/dataplane/port/internal_tag_port"
	"alt/dataplane/usecase/create_tag_set_version_usecase"
	"alt/dataplane/usecase/feeds_in_window_usecase"
	"alt/dataplane/usecase/outbox_usecase"
	"alt/dataplane/usecase/recap_articles_usecase"
	"alt/domain"
	"alt/shared/port/event_publisher_port"
	"alt/shared/port/knowledge_event_port"
	"alt/shared/usecase/create_summary_version_usecase"
)

// HandlerOption configures optional ports on the Handler.
type HandlerOption func(*Handler)

// WithArticleIngestionPorts configures ports for article ingestion (pre-processor) RPCs.
func WithArticleIngestionPorts(
	checkExists internal_article_port.CheckArticleExistsPort,
	createArticle internal_article_port.CreateArticlePort,
	saveSummary internal_article_port.SaveArticleSummaryPort,
	getContent internal_article_port.GetArticleContentPort,
	getFeedID internal_feed_port.GetFeedIDPort,
	listFeedURLs internal_feed_port.ListFeedURLsPort,
) HandlerOption {
	return func(h *Handler) {
		h.checkArticleExists = checkExists
		h.createArticle = createArticle
		h.saveArticleSummary = saveSummary
		h.getArticleContent = getContent
		h.getFeedID = getFeedID
		h.listFeedURLs = listFeedURLs
	}
}

// WithTagCatalogPorts configures ports for tag-generator RPCs.
func WithTagCatalogPorts(
	upsertTags internal_tag_port.UpsertArticleTagsPort,
	batchUpsertTags internal_tag_port.BatchUpsertArticleTagsPort,
	listUntagged internal_tag_port.ListUntaggedArticlesPort,
) HandlerOption {
	return func(h *Handler) {
		h.upsertArticleTags = upsertTags
		h.batchUpsertArticleTags = batchUpsertTags
		h.listUntaggedArticles = listUntagged
	}
}

// WithBatchGetTagsPort wires the BatchGetTagsByArticleIDs port used by
// recap-worker for tag fetches. Replaces the legacy tag-generator
// /api/v1/tags/batch path (ADR-000241 / ADR-000397).
func WithBatchGetTagsPort(p internal_tag_port.BatchGetTagsByArticleIDsPort) HandlerOption {
	return func(h *Handler) {
		h.batchGetTagsByArticleIDs = p
	}
}

// WithSummarizationPorts configures ports for summarization polling RPCs.
func WithSummarizationPorts(
	listUnsummarized internal_article_port.ListUnsummarizedArticlesPort,
	hasUnsummarized internal_article_port.HasUnsummarizedArticlesPort,
) HandlerOption {
	return func(h *Handler) {
		h.listUnsummarized = listUnsummarized
		h.hasUnsummarized = hasUnsummarized
	}
}

// WithEventPublisher configures the event publisher for domain events.
//
// The publisher can be switched off — mq-hub answers IsEnabled from its own
// config — and that is a deployment saying "no notifications", not a wiring
// mistake. The two are told apart here, where the boot log names which one
// this process is in, and at the call site in CreateArticle, which panics on
// the unwired one (CLAUDE.md rule 8 / ADR-000928). Logging rather than
// panicking on nil: the composition root passes this option unconditionally,
// so a nil arriving here is the field being nil, and the process that has to
// stop is the one that reaches the publish, not the one that mounts the mux.
func WithEventPublisher(ep event_publisher_port.EventPublisherPort) HandlerOption {
	return func(h *Handler) {
		h.eventPublisher = ep
		switch {
		case ep == nil:
			h.logger.Error("datahub.event_publisher_unwired",
				"detail", "DataHubService.CreateArticle will panic on its first call")
		case ep.IsEnabled():
			h.logger.Info("datahub.event_publisher_enabled")
		default:
			h.logger.Info("datahub.event_publisher_disabled")
		}
	}
}

// WithBackfillPorts configures ports for backfill-related RPCs.
func WithBackfillPorts(
	getEmptyFeedID internal_feed_port.GetEmptyFeedIDPort,
) HandlerOption {
	return func(h *Handler) {
		h.getEmptyFeedID = getEmptyFeedID
	}
}

// WithKnowledgeEventPort configures the Knowledge Home event append port.
//
// Required, and nil panics: ArticleCreated is the only event that carries an
// article's title and url into the knowledge event log, so a data hub writing
// articles without it fills Knowledge Home with rows that
// SummaryVersionCreated later inserts blank. There is no deployment in which
// that is the intended configuration, which is why "the option was never
// passed" and "the feature is off" must not be the same state (CLAUDE.md rule
// 8 / .claude/rules/di-wiring.md).
func WithKnowledgeEventPort(port knowledge_event_port.AppendKnowledgeEventPort) HandlerOption {
	if port == nil {
		panic("datahubapi: AppendKnowledgeEventPort is required — " +
			"DataHubService.CreateArticle is the Knowledge Home ArticleCreated producer " +
			"for every article pre-processor ingests (see .claude/rules/di-wiring.md)")
	}
	return func(h *Handler) {
		h.knowledgeEventPort = port
	}
}

// WithKnowledgeVersionUsecases configures usecases for Knowledge Home version tracking.
func WithKnowledgeVersionUsecases(
	summaryVersion *create_summary_version_usecase.CreateSummaryVersionUsecase,
	tagSetVersion *create_tag_set_version_usecase.CreateTagSetVersionUsecase,
) HandlerOption {
	return func(h *Handler) {
		h.createSummaryVersionUsecase = summaryVersion
		h.createTagSetVersionUsecase = tagSetVersion
	}
}

// WithSummaryQualityPorts configures ports for quality checker RPCs.
func WithSummaryQualityPorts(
	deleteSummary internal_article_port.DeleteArticleSummaryPort,
	checkSummaryExists internal_article_port.CheckArticleSummaryExistsPort,
	findWithSummaries internal_article_port.FindArticlesWithSummariesPort,
) HandlerOption {
	return func(h *Handler) {
		h.deleteArticleSummary = deleteSummary
		h.checkArticleSummaryExists = checkSummaryExists
		h.findArticlesWithSummaries = findWithSummaries
	}
}

// WithMediaCapabilities wires the media and cache capabilities
// (catalog §2.A / §2.D / §2.E / §2.L / §2.O).
//
// Every argument is required and a nil one panics, which is a departure from
// the optional ports above. Those wire procedures whose consumers are
// separate services that may or may not be deployed; these wire the only route
// two binaries in this repository have to their own database. A data hub that
// started with a nil outbox port would answer ClaimOutboxBatch with
// Unimplemented — the same answer a genuinely retired procedure gives — and
// alt-harvester would tick every five seconds, log nothing unusual, and
// deliver no article to rag-orchestrator until someone noticed the search
// index had stopped moving (CLAUDE.md rule 8, ADR-000928).
func WithMediaCapabilities(
	outbox *outbox_usecase.OutboxUsecase,
	ogImage datahub_capability_port.OgImagePort,
	imageProxyCache datahub_capability_port.ImageProxyCachePort,
	scrapingPolicy datahub_capability_port.ScrapingPolicyPort,
	autoFulltext datahub_capability_port.AutoFulltextPort,
) HandlerOption {
	switch {
	case outbox == nil:
		panic("datahubapi: OutboxUsecase is required — alt-harvester's outbox worker has no other route to outbox_events")
	case ogImage == nil:
		panic("datahubapi: OgImagePort is required — the OG image pipeline has no other route to article_heads")
	case imageProxyCache == nil:
		panic("datahubapi: ImageProxyCachePort is required — the image proxy has no other route to image_proxy_cache")
	case scrapingPolicy == nil:
		panic("datahubapi: ScrapingPolicyPort is required — the article fetch path checks scraping_domains before every body fetch")
	case autoFulltext == nil:
		panic("datahubapi: AutoFulltextPort is required")
	}

	return func(h *Handler) {
		h.outboxUsecase = outbox
		h.ogImage = ogImage
		h.imageProxyCache = imageProxyCache
		h.scrapingPolicy = scrapingPolicy
		h.autoFulltext = autoFulltext
	}
}

// WithArticleCapabilities wires the article capabilities
// (catalog §2.B / §2.C / §2.N).
//
// Nil panics, for the reason WithMediaCapabilities gives at length: after this
// wiring these procedures are alt-backend's only route to the articles table.
// A data hub that started with a nil article read port would answer
// GetArticleByURL with Unimplemented — indistinguishable from a retired
// procedure — and the article page would report "not found" for every article
// in the database while every health check stayed green (CLAUDE.md rule 8,
// ADR-000928).
//
// SaveArticleHead is not here. It is catalog §2.B, but it writes article_heads
// and is served through the OG image port WithMediaCapabilities already wires:
// one table, one port.
func WithArticleCapabilities(
	articleWrite datahub_capability_port.ArticleWritePort,
	articleRead datahub_capability_port.ArticleReadPort,
	knowledgeBackfill datahub_capability_port.KnowledgeBackfillPort,
) HandlerOption {
	switch {
	case articleWrite == nil:
		panic("datahubapi: ArticleWritePort is required — alt-backend has no other route to the articles upsert and its outbox row")
	case articleRead == nil:
		panic("datahubapi: ArticleReadPort is required — every article-serving surface reads through it")
	case knowledgeBackfill == nil:
		panic("datahubapi: KnowledgeBackfillPort is required — the knowledge backfill jobs have no other route to historic articles")
	}

	return func(h *Handler) {
		h.articleWrite = articleWrite
		h.articleRead = articleRead
		h.knowledgeBackfill = knowledgeBackfill
	}
}

// WithFeedCapabilities wires the feed and feed-link capabilities
// (capability catalog §2.F / §2.G / §2.H).
//
// Nil panics, for the same reason the earlier options give: after this wiring
// these procedures are the only route to feed_links, feed_link_availability
// and feeds. A data hub started with a nil feed port would answer
// ListFeedsCursor with Unimplemented — which is also what a retired procedure
// answers — and every user would see an empty feed list while health checks
// stayed green (CLAUDE.md rule 8, ADR-000928).
func WithFeedCapabilities(
	feedLink datahub_capability_port.FeedLinkPort,
	feedLinkAvailability datahub_capability_port.FeedLinkAvailabilityPort,
	feed datahub_capability_port.FeedPort,
) HandlerOption {
	switch {
	case feedLink == nil:
		panic("datahubapi: FeedLinkPort is required — feed registration and the collector's work list have no other route to feed_links")
	case feedLinkAvailability == nil:
		panic("datahubapi: FeedLinkAvailabilityPort is required — without it a dead feed is polled forever")
	case feed == nil:
		panic("datahubapi: FeedPort is required — every feed-serving surface reads through it")
	}

	return func(h *Handler) {
		h.feedLink = feedLink
		h.feedLinkAvailability = feedLinkAvailability
		h.feed = feed
	}
}

// WithReadStateAndTagCapabilities wires the per-user feed state and the tag reads
// (capability catalog §2.I / §2.J).
//
// Nil panics, as in every option before it. After this wiring these procedures
// are the only route to read_status, user_feed_subscriptions, favorite_feeds
// and the tag tables. A data hub started with a nil read-state port would
// answer MarkFeedRead with Unimplemented — indistinguishable from a retired
// procedure — and every reader would find their read marks silently
// discarded while health checks stayed green (CLAUDE.md rule 8, ADR-000928).
func WithReadStateAndTagCapabilities(
	readState datahub_capability_port.ReadStatePort,
	tagRead datahub_capability_port.TagReadPort,
) HandlerOption {
	switch {
	case readState == nil:
		panic("datahubapi: ReadStatePort is required — read marks, subscriptions and favourites have no other route to their tables")
	case tagRead == nil:
		panic("datahubapi: TagReadPort is required — every tag surface, including the on-the-fly generation path, reads through it")
	}

	return func(h *Handler) {
		h.readState = readState
		h.tagRead = tagRead
	}
}

// WithVersionAndStatsCapabilities wires the versioned artifacts and the dashboard
// statistics (capability catalog §2.K / §2.M).
//
// Nil panics, as in every option before it, and the summary-version port is the
// one where a silent Unimplemented would do the most damage. Every summary
// alt-backend writes is accompanied by a SummaryVersionCreated event appended
// to knowledge-sovereign by the caller; if the version write behind it
// answered Unimplemented, the events would keep flowing and sovereign would
// accumulate references to versions that were never persisted. Nothing would
// look broken until somebody replayed the log (CLAUDE.md rule 8, ADR-000928).
func WithVersionAndStatsCapabilities(
	summaryVersion datahub_capability_port.SummaryVersionPort,
	tagSetVersion datahub_capability_port.TagSetVersionPort,
	stats datahub_capability_port.StatsPort,
) HandlerOption {
	switch {
	case summaryVersion == nil:
		panic("datahubapi: SummaryVersionPort is required — summary_versions has no other route, and the knowledge events describing those versions are appended regardless")
	case tagSetVersion == nil:
		panic("datahubapi: TagSetVersionPort is required — tag_set_versions has no other route, and TagSetVersionCreated is appended regardless")
	case stats == nil:
		panic("datahubapi: StatsPort is required — every dashboard count and the trend chart read through it")
	}

	return func(h *Handler) {
		h.summaryVersion = summaryVersion
		h.tagSetVersion = tagSetVersion
		h.stats = stats
	}
}

// WithTagTrailCapabilities wires the two capabilities that close the data plane:
// the Tag Trail's paged reads (catalog §2.J) and the recall rail's article fallback (§2.C).
//
// These are the last two, and after them alt-backend has no database pool at
// all. Both refuse nil for the same reason every option before them did, but
// the failure they prevent is quieter than most: neither of these reads
// returning nothing looks like an error to its caller. An empty Tag Trail page
// renders as "this tag has no articles", and a recall candidate with no
// article behind it is simply skipped. A nil port here would therefore produce
// a working-looking product with two features missing, which is exactly the
// state CLAUDE.md rule 8 exists to make impossible.
func WithTagTrailCapabilities(
	tagTrail datahub_capability_port.TagTrailPort,
	articleRef datahub_capability_port.ArticleRefPort,
) HandlerOption {
	switch {
	case tagTrail == nil:
		panic("datahubapi: TagTrailPort is required — an unwired Tag Trail answers every tag with an empty page, which the UI renders as 'no articles' rather than as a fault")
	case articleRef == nil:
		panic("datahubapi: ArticleRefPort is required — an unwired recall fallback silently drops exactly the items the fallback exists to render")
	}

	return func(h *Handler) {
		h.tagTrail = tagTrail
		h.articleRef = articleRef
	}
}

// --- RAG Tool Operations (ADR-000617) ---

// fetchTagCloudPort is the port interface for fetching tag cloud data.
type fetchTagCloudPort interface {
	Execute(ctx context.Context, limit int) ([]*domain.TagCloudItem, error)
}

// fetchArticlesByTagPort is the port interface for fetching articles by tag name.
type fetchArticlesByTagPort interface {
	ExecuteByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error)
}

// WithRAGToolPorts configures ports for RAG tool RPCs (ADR-000617).
func WithRAGToolPorts(
	tagCloud fetchTagCloudPort,
	articlesByTag fetchArticlesByTagPort,
) HandlerOption {
	return func(h *Handler) {
		h.fetchTagCloudPort = tagCloud
		h.fetchArticlesByTagPort = articlesByTag
	}
}

// recapArticlesUsecase is the minimal interface for paginated article window
// fetch. The concrete usecase lives at alt/usecase/recap_articles_usecase.
type recapArticlesUsecase interface {
	Execute(ctx context.Context, input recap_articles_usecase.Input) (*domain.RecapArticlesPage, error)
}

// WithRecapArticlesUsecase wires the recap-worker's paginated article window
// fetch (service-to-service RPC ListRecapArticles).
func WithRecapArticlesUsecase(uc recapArticlesUsecase) HandlerOption {
	return func(h *Handler) {
		h.recapArticlesUsecase = uc
	}
}

// feedsInWindowUsecase is the minimal interface for paginated feed window
// fetch. The concrete usecase lives at alt/dataplane/usecase/feeds_in_window_usecase.
type feedsInWindowUsecase interface {
	Execute(ctx context.Context, input feeds_in_window_usecase.Input) (*domain.FeedsInWindowPage, error)
}

// WithFeedsInWindowUsecase wires the recap-worker's paginated feed window
// fetch (service-to-service RPC ListFeedsInWindow).
func WithFeedsInWindowUsecase(uc feedsInWindowUsecase) HandlerOption {
	return func(h *Handler) {
		h.feedsInWindowUsecase = uc
	}
}
