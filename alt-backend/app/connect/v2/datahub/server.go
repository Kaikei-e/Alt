// Package datahub provides the Connect-RPC server for cmd/datahub's
// mutual-TLS listener.
//
// It is a package of its own rather than another function in alt/connect/v2 so
// that the two surfaces do not link each other. cmd/backend must not contain
// DataHubService's handler at all — not merely leave it unmounted — and
// cmd/datahub must not contain the browser-facing handlers. Sharing a package
// would compile both into both.
//
// There is deliberately no constructor anywhere that serves the user, admin
// and service-to-service surfaces from one mux. The listener this replaced
// (CreateMTLSConnectServer on :9443) did exactly that, and whether it verified
// the caller's certificate at all depended on an environment variable that
// defaulted to "no".
package datahub

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	"alt/config"
	"alt/connect/v2/middleware"
	"alt/connect/v2/muxutil"
	"alt/dataplane/connect/datahubapi"
	datahubdi "alt/di/datahub"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
)

// SetupConnectHandlers registers the service-to-service API cmd/datahub serves
// behind mutual TLS: services.datahub.v1.DataHubService, and nothing else.
//
// During the migration this mux carried a second mount,
// services.backend.v1.BackendInternalService, so that peers migrated one PR at
// a time to ADR-000954 D7's namespace and the ones that had not moved yet kept
// working. That legacy mount was removed once all five consumers were across. What is
// left is one name for one surface — a call on the retired path now finds
// nothing here, which is what makes "the data plane has a single door" a
// property of the code rather than of the deployment.
//
// It takes *di/datahub.DataHubComponents rather than the backend's component set: the
// event publisher, the Kratos client and the recap/tag-set read models it
// needs are built by that binary alone, so a backend handler cannot reach them
// even by accident (CLAUDE.md rule 8 — absent field, compile error).
func SetupConnectHandlers(mux *http.ServeMux, container *datahubdi.DataHubComponents, cfg *config.Config, logger *slog.Logger) {
	cancelInterceptor := middleware.NewContextCancelInterceptor(logger)

	datahubOpts := connect.WithInterceptors(
		cancelInterceptor.Interceptor(),
	)
	// The two capabilities ADR-000954 D6 absorbs from /v1/internal are checked
	// here, on the concrete container fields, rather than inside NewHandler.
	// FetchRecentArticlesUsecase is a pointer: a nil one stored in the
	// handler's interface field is not nil as an interface value, so the
	// handler's own guard would let it through and the procedure would fail on
	// the first call instead of at boot.
	// SovereignClient is checked here for the same reason, and it is a pointer
	// too: CreateArticle is one of the two producers of Knowledge Home's
	// ArticleCreated event — the only event carrying an article's title and url
	// into the event log — and a data hub writing articles without it fills
	// Home with rows that SummaryVersionCreated later inserts blank.
	switch {
	case container.KratosClient == nil:
		panic("datahub: DataHubComponents.KratosClient is nil — DataHubService.GetSystemUser has no identity source")
	case container.FetchRecentArticlesUsecase == nil:
		panic("datahub: DataHubComponents.FetchRecentArticlesUsecase is nil — DataHubService.ListRecentArticles has no read behind it")
	case container.SovereignClient == nil:
		panic("datahub: DataHubComponents.SovereignClient is nil — DataHubService.CreateArticle has nowhere to append ArticleCreated")
	}

	articleGw := container.ArticleCatalogGateway
	feedGw := container.FeedCatalogGateway
	tagGw := container.TagCatalogGateway
	datahubHandler := datahubapi.NewHandler(
		articleGw, articleGw, articleGw, articleGw, articleGw,
		container.KratosClient,
		container.FetchRecentArticlesUsecase,
		logger,
		datahubapi.WithArticleIngestionPorts(articleGw, articleGw, articleGw, articleGw, feedGw, feedGw),
		datahubapi.WithTagCatalogPorts(tagGw, tagGw, tagGw),
		datahubapi.WithBatchGetTagsPort(tagGw),
		datahubapi.WithSummaryQualityPorts(articleGw, articleGw, articleGw),
		datahubapi.WithSummarizationPorts(articleGw, articleGw),
		datahubapi.WithBackfillPorts(feedGw),
		datahubapi.WithEventPublisher(container.EventPublisher),
		datahubapi.WithKnowledgeVersionUsecases(container.CreateSummaryVersionUsecase, container.CreateTagSetVersionUsecase),
		datahubapi.WithKnowledgeEventPort(container.SovereignClient),
		datahubapi.WithRAGToolPorts(container.FetchTagCloudUsecase, container.FetchArticlesByTagUsecase),
		datahubapi.WithRecapArticlesUsecase(container.RecapArticlesUsecase),
		datahubapi.WithFeedsInWindowUsecase(container.FeedsInWindowUsecase),
		// Media and cache capabilities. Unlike the optional options above, every
		// argument here is required and WithMediaCapabilities panics on a nil
		// one: these are the only route alt-backend and alt-harvester have to
		// the outbox, article_heads, the image cache and the scraping policy.
		datahubapi.WithMediaCapabilities(
			container.OutboxUsecase,
			container.OgImageGateway,
			container.ImageProxyCacheGateway,
			container.ScrapingPolicyGateway,
			container.AutoFulltextGateway,
		),
		// Article capabilities, same rule: after this wiring alt-backend
		// has no database pool for articles, so a nil here would make every
		// article surface answer Unimplemented.
		datahubapi.WithArticleCapabilities(
			container.ArticleWriteGateway,
			container.ArticleReadGateway,
			container.KnowledgeBackfillGateway,
		),
		// Feed and feed-link capabilities, same rule once more: feed_links,
		// feed_link_availability and feeds. A nil feed port would leave every
		// user looking at an empty feed list, and a nil availability port
		// would leave alt-harvester polling dead feeds forever — both while
		// the process reported healthy.
		datahubapi.WithFeedCapabilities(
			container.FeedLinkGateway,
			container.FeedLinkAvailabilityGateway,
			container.FeedGateway,
		),
		// Read-state and tag-read capabilities, same rule again: read_status,
		// user_feed_subscriptions, favorite_feeds and the tag tables. A nil
		// read-state port would make every read mark and every star vanish
		// without an error anyone could see, and a nil tag port would make
		// every article look untagged — which the on-the-fly path reads as
		// "generate some" and would turn into an mq-hub request per view.
		datahubapi.WithReadStateAndTagCapabilities(
			container.ReadStateGateway,
			container.TagReadGateway,
		),
		// Versioned artifact and stats capabilities, and the rule holds to the end:
		// summary_versions, tag_set_versions and every dashboard count. A nil
		// version port is the worst of the capability groups, because the knowledge
		// events describing those versions are appended by the caller either
		// way — sovereign would fill with references to versions that were
		// never written, and nothing would look wrong until a replay.
		datahubapi.WithVersionAndStatsCapabilities(
			container.SummaryVersionCapabilityGateway,
			container.TagSetVersionCapabilityGateway,
			container.StatsGateway,
		),
		// Tag Trail and article reference capabilities, and the last application of the rule:
		// the Tag Trail's paged reads and the recall rail's article fallback.
		// These two are the quietest failures of the capability groups — an unwired
		// Tag Trail renders "no articles" and an unwired fallback drops
		// exactly the items it exists to rescue, both with a 200 — which is
		// why they refuse nil at construction like the rest.
		datahubapi.WithTagTrailCapabilities(
			container.TagTrailGateway,
			container.ArticleRefGateway,
		),
		// Web Push storage, and the rule once more: push_subscriptions is the
		// only route alt.push.v1.PushService has, so a nil here would let a
		// user grant notification permission and watch the toggle fail with
		// nothing in any log saying the storage was never wired. The delivery
		// queue is quieter still — an unwired one answers a dispatcher's claim
		// exactly the way a drained one does.
		datahubapi.WithPushCapabilities(
			container.PushSubscriptionGateway,
			container.PushDeliveryUsecase,
		),
	)
	datahubPath, datahubServiceHandler := datahubv1connect.NewDataHubServiceHandler(datahubHandler, datahubOpts)
	mux.Handle(datahubPath, datahubServiceHandler)

	// One line at startup naming the mount, so "which namespace is this
	// process answering on" is answerable from the boot log rather than only
	// from whichever peer happens to call next (CLAUDE.md rule 8).
	logger.Info("datahub_namespaces.wiring",
		"current", datahubPath,
		"retired", "/services.backend.v1.BackendInternalService/",
		"retired_in", "ADR-000954 Wave 2-C",
	)

	// And one naming the ArticleCreated producer, so an operator can tell from
	// the boot log whether this process appends Knowledge Home events for the
	// articles it writes. There is no disabled counterpart on purpose: the
	// checks above make an unwired sink a process that does not start, and
	// NewDataHubComponents already refuses a sovereign client that would no-op
	// (CLAUDE.md rule 8).
	logger.Info("datahub.article_created_producer_enabled",
		"procedure", datahubPath+"CreateArticle",
		"sink", "knowledge-sovereign AppendKnowledgeEvent",
		"sovereign_enabled", container.SovereignClient.Enabled(),
	)
}

// CreateServer builds the Connect-RPC handler for data-hub's mutual-TLS
// listener: DataHubService and /health, and nothing else.
//
// No h2c wrapper: this handler is only ever mounted on the TLS listener built
// by tlsutil.NewMTLSHTTPServer, which negotiates HTTP/2 through ALPN. Cleartext
// HTTP/2 is a property of the plaintext listeners, and is set there via
// http.Server.Protocols.
func CreateServer(container *datahubdi.DataHubComponents, cfg *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	muxutil.RegisterHealth(mux)
	SetupConnectHandlers(mux, container, cfg, logger)

	return mux
}
