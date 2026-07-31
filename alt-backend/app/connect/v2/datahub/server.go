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
	"alt/di"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
)

// SetupConnectHandlers registers the service-to-service API cmd/datahub serves
// behind mutual TLS: alt.datahub.v1.DataHubService, and nothing else.
//
// Through Wave 2 this mux carried a second mount,
// services.backend.v1.BackendInternalService, so that peers migrated one PR at
// a time to ADR-000954 D7's namespace and the ones that had not moved yet kept
// working. Wave 2-C removed it once all five consumers were across. What is
// left is one name for one surface — a call on the retired path now finds
// nothing here, which is what makes "the data plane has a single door" a
// property of the code rather than of the deployment.
//
// It takes *di.DataHubComponents rather than the backend's component set: the
// event publisher, the Kratos client and the recap/tag-set read models it
// needs are built by that binary alone, so a backend handler cannot reach them
// even by accident (CLAUDE.md rule 8 — absent field, compile error).
func SetupConnectHandlers(mux *http.ServeMux, container *di.DataHubComponents, cfg *config.Config, logger *slog.Logger) {
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
	switch {
	case container.KratosClient == nil:
		panic("datahub: DataHubComponents.KratosClient is nil — DataHubService.GetSystemUser has no identity source")
	case container.FetchRecentArticlesUsecase == nil:
		panic("datahub: DataHubComponents.FetchRecentArticlesUsecase is nil — DataHubService.ListRecentArticles has no read behind it")
	}

	gw := container.InternalArticleGateway
	datahubHandler := datahubapi.NewHandler(
		gw, gw, gw, gw, gw,
		container.KratosClient,
		container.FetchRecentArticlesUsecase,
		logger,
		datahubapi.WithPhase2Ports(gw, gw, gw, gw, gw, gw),
		datahubapi.WithPhase3Ports(gw, gw, gw),
		datahubapi.WithBatchGetTagsPort(gw),
		datahubapi.WithPhase4Ports(gw, gw, gw),
		datahubapi.WithSummarizationPorts(gw, gw),
		datahubapi.WithBackfillPorts(gw),
		datahubapi.WithEventPublisher(container.EventPublisher),
		datahubapi.WithKnowledgeVersionUsecases(container.CreateSummaryVersionUsecase, container.CreateTagSetVersionUsecase),
		datahubapi.WithKnowledgeEventPort(container.SovereignClient),
		datahubapi.WithRAGToolPorts(container.FetchTagCloudUsecase, container.FetchArticlesByTagUsecase),
		datahubapi.WithRecapArticlesUsecase(container.RecapArticlesUsecase),
		// ADR-000954 Wave 3 batch 1. Unlike the phase options above, every
		// argument here is required and WithWave3Capabilities panics on a nil
		// one: these are the only route alt-backend and alt-harvester have to
		// the outbox, article_heads, the image cache and the scraping policy.
		datahubapi.WithWave3Capabilities(
			container.OutboxUsecase,
			container.OgImageGateway,
			container.ImageProxyCacheGateway,
			container.ScrapingPolicyGateway,
			container.AutoFulltextGateway,
		),
		// ADR-000954 Wave 3 batch 2, same rule: after this batch alt-backend
		// has no database pool for articles, so a nil here would make every
		// article surface answer Unimplemented.
		datahubapi.WithWave3Batch2Capabilities(
			container.ArticleWriteGateway,
			container.ArticleReadGateway,
			container.KnowledgeBackfillGateway,
		),
		// ADR-000954 Wave 3 batch 3, same rule once more: feed_links,
		// feed_link_availability and feeds. A nil feed port would leave every
		// user looking at an empty feed list, and a nil availability port
		// would leave alt-harvester polling dead feeds forever — both while
		// the process reported healthy.
		datahubapi.WithWave3Batch3Capabilities(
			container.FeedLinkGateway,
			container.FeedLinkAvailabilityGateway,
			container.FeedGateway,
		),
		// ADR-000954 Wave 3 batch 4, same rule again: read_status,
		// user_feed_subscriptions, favorite_feeds and the tag tables. A nil
		// read-state port would make every read mark and every star vanish
		// without an error anyone could see, and a nil tag port would make
		// every article look untagged — which the on-the-fly path reads as
		// "generate some" and would turn into an mq-hub request per view.
		datahubapi.WithWave3Batch4Capabilities(
			container.ReadStateGateway,
			container.TagReadGateway,
		),
		// ADR-000954 Wave 3 batch 5, and the rule holds to the end:
		// summary_versions, tag_set_versions and every dashboard count. A nil
		// version port is the worst of the five batches, because the knowledge
		// events describing those versions are appended by the caller either
		// way — sovereign would fill with references to versions that were
		// never written, and nothing would look wrong until a replay.
		datahubapi.WithWave3Batch5Capabilities(
			container.SummaryVersionCapabilityGateway,
			container.TagSetVersionCapabilityGateway,
			container.StatsGateway,
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
}

// CreateServer builds the Connect-RPC handler for data-hub's mutual-TLS
// listener: DataHubService and /health, and nothing else.
func CreateServer(container *di.DataHubComponents, cfg *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	muxutil.RegisterHealth(mux)
	SetupConnectHandlers(mux, container, cfg, logger)

	return muxutil.WithH2C(mux)
}
