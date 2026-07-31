// Package v2 provides Connect-RPC server setup and configuration.
package v2

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"alt/gen/proto/alt/admin_monitor/v1/adminmonitorv1connect"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"alt/gen/proto/alt/knowledge_home/v1/knowledgehomev1connect"
	"alt/gen/proto/alt/knowledge_trail/v1/knowledgetrailv1connect"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
	"alt/gen/proto/alt/recap/v2/recapv2connect"
	"alt/gen/proto/alt/rss/v2/rssv2connect"
	"alt/gen/proto/alt/search/v2/searchv2connect"

	"alt/config"
	"alt/connect/v2/codec"
	"alt/connect/v2/middleware"
	"alt/connect/v2/muxutil"
	"alt/di"
	recapinternal "alt/internal/recap"
	"alt/orchestrator/connect/v2/admin_monitor"
	"alt/orchestrator/connect/v2/articles"
	"alt/orchestrator/connect/v2/augur"
	"alt/orchestrator/connect/v2/feeds"
	global_search "alt/orchestrator/connect/v2/global_search"
	knowledge_home "alt/orchestrator/connect/v2/knowledge_home"
	"alt/orchestrator/connect/v2/knowledge_home_admin"
	knowledge_trail "alt/orchestrator/connect/v2/knowledge_trail"
	"alt/orchestrator/connect/v2/morning_letter"
	"alt/orchestrator/connect/v2/recap"
	"alt/orchestrator/connect/v2/rss"
)

// SetupConnectHandlers registers the browser-facing Connect-RPC handlers with
// the HTTP mux. Every service registered here runs behind the JWT auth
// interceptor. Service-to-service and admin surfaces belong on the internal
// mux instead — see SetupOperatorConnectHandlers for the admin services and
// alt/connect/v2/datahub for the service-to-service ones.
func SetupConnectHandlers(mux *http.ServeMux, container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) {
	// Create interceptors
	cancelInterceptor := middleware.NewContextCancelInterceptor(logger)
	authInterceptor := middleware.NewAuthInterceptor(logger, cfg)
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		logger.Warn("Failed to create OTel interceptor, proceeding without tracing", "error", err)
	}

	// Handler options with interceptors (cancelInterceptor outermost: catches all errors)
	interceptors := []connect.Interceptor{
		cancelInterceptor.Interceptor(),
		authInterceptor.Interceptor(),
	}
	if otelInterceptor != nil {
		interceptors = append(interceptors, otelInterceptor)
	}
	opts := connect.WithInterceptors(interceptors...)

	// Register Feed service
	feedHandler := feeds.NewHandler(feeds.FeedHandlerDeps{
		CachedFeedList:           container.CachedFeedListUsecase,
		FetchReadFeedsCursor:     container.FetchReadFeedsListCursorUsecase,
		FetchFavoriteFeedsCursor: container.FetchFavoriteFeedsListCursorUsecase,
		FeedSearch:               container.FeedSearchUsecase,
		ListSubscriptions:        container.ListSubscriptionsUsecase,
		ArticlesReadingStatus:    container.ArticlesReadingStatusUsecase,
		Subscribe:                container.SubscribeUsecase,
		Unsubscribe:              container.UnsubscribeUsecase,
		FeedAmount:               container.FeedAmountUsecase,
		UnsummarizedCount:        container.UnsummarizedArticlesCountUsecase,
		SummarizedCount:          container.SummarizedArticlesCountUsecase,
		TotalCount:               container.TotalArticlesCountUsecase,
		TodayUnreadCount:         container.TodayUnreadArticlesCountUsecase,
		ArticleStore:             container.Infra.ArticleStoreGateway,
		SummaryStore:             container.Infra.FeedGateway,
		FeedTagStore:             container.Infra.TagGateway,
		PreProcessorClient:       container.PreProcessorConnectClient,
		CreateSummaryVersion:     container.CreateSummaryVersionUsecase,
		ImageProxy:               container.ImageProxyUsecase,
	}, cfg, logger)
	feedPath, feedServiceHandler := feedsv2connect.NewFeedServiceHandler(feedHandler, opts)
	mux.Handle(feedPath, feedServiceHandler)
	logger.Info("Registered Connect-RPC FeedService", "path", feedPath)

	// Register Article service
	articleHandler := articles.NewHandler(articles.ArticleHandlerDeps{
		OgImageURLs:             container.Infra.OgImageGateway,
		ArchiveArticle:          container.ArchiveArticleUsecase,
		Article:                 container.ArticleUsecase,
		FetchArticlesByTag:      container.FetchArticlesByTagUsecase,
		FetchArticlesCursor:     container.FetchArticlesCursorUsecase,
		FetchArticleSummary:     container.FetchArticleSummaryUsecase,
		FetchArticleTags:        container.FetchArticleTagsUsecase,
		FetchInoreaderSummary:   container.FetchInoreaderSummaryUsecase,
		FetchLatestArticle:      container.FetchLatestArticleUsecase,
		FetchRandomSubscription: container.FetchRandomSubscriptionUsecase,
		FetchTagCloud:           container.FetchTagCloudUsecase,
		GetArticleSourceURL:     container.GetArticleSourceURLUsecase,
		ImageProxy:              container.ImageProxyUsecase,
		StreamArticleTags:       container.StreamArticleTagsUsecase,
	}, cfg, logger)
	articlePath, articleServiceHandler := articlesv2connect.NewArticleServiceHandler(articleHandler, opts)
	mux.Handle(articlePath, articleServiceHandler)
	logger.Info("Registered Connect-RPC ArticleService", "path", articlePath)

	// Register RSS service
	rssHandler := rss.NewHandler(container, cfg, logger)
	rssPath, rssServiceHandler := rssv2connect.NewRSSServiceHandler(rssHandler, opts)
	mux.Handle(rssPath, rssServiceHandler)
	logger.Info("Registered Connect-RPC RSSService", "path", rssPath)

	// Register Augur service (uses Connect-RPC to communicate with rag-orchestrator)
	augurHandler := augur.NewHandler(container.RetrieveContextUsecase, container.RagConnectClient, logger)
	augurPath, augurServiceHandler := augurv2connect.NewAugurServiceHandler(augurHandler, opts)
	mux.Handle(augurPath, augurServiceHandler)
	logger.Info("Registered Connect-RPC AugurService", "path", augurPath)

	// Register MorningLetter services (chat + read)
	morningLetterHandler := morning_letter.NewHandler(container.StreamChatPort, container.MorningLetterUsecase, logger)
	morningLetterPath, morningLetterServiceHandler := morningletterv2connect.NewMorningLetterServiceHandler(morningLetterHandler, opts)
	mux.Handle(morningLetterPath, morningLetterServiceHandler)
	logger.Info("Registered Connect-RPC MorningLetterService", "path", morningLetterPath)

	// Register MorningLetterReadService (document-oriented read APIs)
	readPath, readServiceHandler := morningletterv2connect.NewMorningLetterReadServiceHandler(morningLetterHandler, opts)
	mux.Handle(readPath, readServiceHandler)
	logger.Info("Registered Connect-RPC MorningLetterReadService", "path", readPath)

	// Register Recap service
	clusterDraftLoader := recapinternal.NewClusterDraftLoader(cfg.Recap.ClusterDraftPath)
	recapHandler := recap.NewHandler(container.RecapUsecase, clusterDraftLoader, logger)
	recapPath, recapServiceHandler := recapv2connect.NewRecapServiceHandler(recapHandler, opts)
	mux.Handle(recapPath, recapServiceHandler)
	logger.Info("Registered Connect-RPC RecapService", "path", recapPath)

	// Register KnowledgeHome service
	knowledgeHomeHandler := knowledge_home.NewHandler(
		container.GetKnowledgeHomeUsecase,
		container.TrackHomeSeenUsecase,
		container.TrackHomeActionUsecase,
		container.RecallRailUsecase,
		container.RecallSnoozeUsecase,
		container.RecallDismissUsecase,
		container.CreateLensUsecase,
		container.UpdateLensUsecase,
		container.ListLensesUsecase,
		container.SelectLensUsecase,
		container.ArchiveLensUsecase,
		container.SovereignClient,
		container.SovereignClient,
		container.SovereignClient,
		container.SovereignClient,
		container.FeatureFlagGateway,
		container.KnowledgeHomeMetrics,
		logger,
	)
	khPath, khServiceHandler := knowledgehomev1connect.NewKnowledgeHomeServiceHandler(knowledgeHomeHandler, opts)
	mux.Handle(khPath, khServiceHandler)
	logger.Info("Registered Connect-RPC KnowledgeHomeService", "path", khPath)

	// Register KnowledgeTrailService — the footprint spine read API.
	// Rule 8: surface the act_outcome producer wiring state loudly at startup
	// so a missing DI edge is visible immediately, not as silently absent
	// dwell outcomes weeks later (PM-2026-045 / ADR-000928).
	logger.Info("trail.act_outcome_producer.wiring",
		"enabled", container.EmitTrailOutcomeUsecase != nil)
	logger.Info("trail.search_usecase.wiring",
		"enabled", container.SearchTrailUsecase != nil)
	logger.Info("trail.item_branches_usecase.wiring",
		"enabled", container.GetItemBranchesUsecase != nil)
	knowledgeTrailHandler := knowledge_trail.NewHandler(container.GetKnowledgeTrailUsecase, container.ResolveTrailBranchUsecase, container.EmitTrailOutcomeUsecase, container.SearchTrailUsecase, container.GetItemBranchesUsecase, container.ImageProxyUsecase, logger)
	ktPath, ktServiceHandler := knowledgetrailv1connect.NewKnowledgeTrailServiceHandler(knowledgeTrailHandler, opts)
	mux.Handle(ktPath, ktServiceHandler)
	logger.Info("Registered Connect-RPC KnowledgeTrailService", "path", ktPath)

	// Register GlobalSearchService
	if container.Search != nil {
		globalSearchHandler := global_search.NewHandler(container.Search.GlobalSearchUsecase, logger)
		gsPath, gsServiceHandler := searchv2connect.NewGlobalSearchServiceHandler(globalSearchHandler, opts)
		mux.Handle(gsPath, gsServiceHandler)
		logger.Info("Registered Connect-RPC GlobalSearchService", "path", gsPath)
	}
}

// SetupOperatorConnectHandlers registers the admin Connect-RPC handlers that
// cmd/backend serves on its loopback operator listener.
//
// These used to share a mux with the data-plane service, which meant one
// listener answered to two unrelated access controls: reachability (loopback
// bind) for the admin surfaces and a client certificate for the
// service-to-service one. They are separate binaries now — the
// service-to-service surface lives in SetupDataHubConnectHandlers — so neither
// may re-acquire the other's services.
func SetupOperatorConnectHandlers(mux *http.ServeMux, container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) {
	cancelInterceptor := middleware.NewContextCancelInterceptor(logger)

	// The custom JSON codec replaces Connect-RPC's default protojson
	// marshaler so proto3 default-valued scalars (zero counters, false
	// flags) stay present in the JSON response. The admin Hurl
	// boundary contracts (e.g. 72-admin-emit-article-url-backfill.hurl)
	// assert that every field of the response envelope round-trips,
	// which is impossible if the wire encoder strips zero values.
	adminOpts := connect.WithInterceptors(
		cancelInterceptor.Interceptor(),
	)
	khAdminHandler := knowledge_home_admin.NewHandler(
		container.KnowledgeBackfillUsecase,
		container.KnowledgeURLBackfillUsecase,
		container.KnowledgeProjectionHealthUsecase,
		container.ReprojectUsecase,
		container.SLOUsecase,
		container.AuditUsecase,
		container.MetricsUsecase,
		&cfg.KnowledgeHome,
		logger,
	)
	khAdminPath, khAdminServiceHandler := knowledgehomev1connect.NewKnowledgeHomeAdminServiceHandler(
		khAdminHandler,
		adminOpts,
		connect.WithCodec(codec.EmitUnpopulatedJSONCodec{}),
		connect.WithCodec(codec.EmitUnpopulatedJSONCharsetUTF8Codec{}),
	)
	mux.Handle(khAdminPath, khAdminServiceHandler)
	logger.Info("Registered Connect-RPC KnowledgeHomeAdminService", "path", khAdminPath)

	// Register AdminMonitorService (Prometheus-backed observability for Admin UI).
	// Gated by config.AdminMonitor.Enabled so production rollout is flag-controlled.
	// The BFF validates the user JWT + admin role before forwarding here.
	if container.AdminMonitor != nil && container.AdminMonitor.Enabled && container.AdminMonitor.Facade != nil {
		amHandler := admin_monitor.NewHandler(container.AdminMonitor.Facade, logger)
		amPath, amServiceHandler := adminmonitorv1connect.NewAdminMonitorServiceHandler(amHandler, adminOpts)
		mux.Handle(amPath, amServiceHandler)
		logger.Info("Registered Connect-RPC AdminMonitorService", "path", amPath)
	} else {
		logger.Info("AdminMonitorService disabled (config.AdminMonitor.Enabled=false)")
	}
}

// CreateConnectServer creates the browser-facing Connect-RPC server. This is
// the handler behind the published plaintext port, so it carries only
// JWT-guarded user services.
//
// Cleartext HTTP/2, which Connect's gRPC protocol needs on a plaintext
// listener, is enabled by bootstrap.NewConnectServer via http.Server.Protocols
// — a server field, so it cannot be expressed here.
func CreateConnectServer(container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	muxutil.RegisterHealth(mux)
	SetupConnectHandlers(mux, container, cfg, logger)

	return mux
}

// CreateOperatorConnectServer creates the Connect-RPC server for
// cmd/backend's loopback operator listener: admin surfaces only.
//
// Cleartext HTTP/2 is enabled by bootstrap.NewServiceServer, which is what
// mounts this handler.
func CreateOperatorConnectServer(container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	muxutil.RegisterHealth(mux)
	SetupOperatorConnectHandlers(mux, container, cfg, logger)

	return mux
}
