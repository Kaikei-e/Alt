// Package v2 provides Connect-RPC server setup and configuration.
package v2

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"alt/gen/proto/alt/admin_monitor/v1/adminmonitorv1connect"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"alt/gen/proto/alt/knowledge_home/v1/knowledgehomev1connect"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
	"alt/gen/proto/alt/recap/v2/recapv2connect"
	"alt/gen/proto/alt/rss/v2/rssv2connect"
	"alt/gen/proto/alt/search/v2/searchv2connect"
	"alt/gen/proto/services/backend/v1/backendv1connect"

	"alt/config"
	"alt/connect/v2/admin_monitor"
	"alt/connect/v2/articles"
	"alt/connect/v2/augur"
	"alt/connect/v2/feeds"
	global_search "alt/connect/v2/global_search"
	internalhandler "alt/connect/v2/internal"
	knowledge_home "alt/connect/v2/knowledge_home"
	"alt/connect/v2/knowledge_home_admin"
	"alt/connect/v2/middleware"
	"alt/connect/v2/morning_letter"
	"alt/connect/v2/recap"
	"alt/connect/v2/rss"
	"alt/di"
	recapinternal "alt/internal/recap"
)

// SetupConnectHandlers registers all Connect-RPC handlers with the HTTP mux.
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
		AltDBRepository:          container.AltDBRepository,
		PreProcessorClient:       container.PreProcessorConnectClient,
		CreateSummaryVersion:     container.CreateSummaryVersionUsecase,
		ImageProxy:               container.ImageProxyUsecase,
	}, cfg, logger)
	feedPath, feedServiceHandler := feedsv2connect.NewFeedServiceHandler(feedHandler, opts)
	mux.Handle(feedPath, feedServiceHandler)
	logger.Info("Registered Connect-RPC FeedService", "path", feedPath)

	// Register Article service
	articleHandler := articles.NewHandler(articles.ArticleHandlerDeps{
		AltDBRepository:         container.AltDBRepository,
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

	// Register KnowledgeHomeAdminService (service-to-service API). Auth is
	// established at the TLS transport layer (mTLS peer-identity).
	adminOpts := connect.WithInterceptors(
		cancelInterceptor.Interceptor(),
	)
	khAdminHandler := knowledge_home_admin.NewHandler(
		container.KnowledgeBackfillUsecase,
		container.KnowledgeProjectionHealthUsecase,
		container.ReprojectUsecase,
		container.SLOUsecase,
		container.AuditUsecase,
		container.MetricsUsecase,
		&cfg.KnowledgeHome,
		logger,
	)
	khAdminPath, khAdminServiceHandler := knowledgehomev1connect.NewKnowledgeHomeAdminServiceHandler(khAdminHandler, adminOpts)
	mux.Handle(khAdminPath, khAdminServiceHandler)
	logger.Info("Registered Connect-RPC KnowledgeHomeAdminService", "path", khAdminPath)

	// Register AdminMonitorService (Prometheus-backed observability for Admin UI).
	// Gated by config.AdminMonitor.Enabled so production rollout is flag-controlled.
	// Auth: BFF validates the user JWT + admin role; service-to-service auth is
	// established at the TLS transport layer.
	if container.AdminMonitor != nil && container.AdminMonitor.Enabled && container.AdminMonitor.Facade != nil {
		amHandler := admin_monitor.NewHandler(container.AdminMonitor.Facade, logger)
		amPath, amServiceHandler := adminmonitorv1connect.NewAdminMonitorServiceHandler(amHandler, adminOpts)
		mux.Handle(amPath, amServiceHandler)
		logger.Info("Registered Connect-RPC AdminMonitorService", "path", amPath)
	} else {
		logger.Info("AdminMonitorService disabled (config.AdminMonitor.Enabled=false)")
	}

	// Register GlobalSearchService
	if container.Search != nil {
		globalSearchHandler := global_search.NewHandler(container.Search.GlobalSearchUsecase, logger)
		gsPath, gsServiceHandler := searchv2connect.NewGlobalSearchServiceHandler(globalSearchHandler, opts)
		mux.Handle(gsPath, gsServiceHandler)
		logger.Info("Registered Connect-RPC GlobalSearchService", "path", gsPath)
	}

	// Register BackendInternalService (service-to-service API). Auth is
	// established at the TLS transport layer (mTLS peer-identity).
	internalOpts := connect.WithInterceptors(
		cancelInterceptor.Interceptor(),
	)
	gw := container.InternalArticleGateway
	internalHandler := internalhandler.NewHandler(
		gw, gw, gw, gw, gw,
		logger,
		internalhandler.WithPhase2Ports(gw, gw, gw, gw, gw, gw),
		internalhandler.WithPhase3Ports(gw, gw, gw),
		internalhandler.WithPhase4Ports(gw, gw, gw),
		internalhandler.WithSummarizationPorts(gw, gw),
		internalhandler.WithBackfillPorts(gw),
		internalhandler.WithEventPublisher(container.EventPublisher),
		internalhandler.WithKnowledgeVersionUsecases(container.CreateSummaryVersionUsecase, container.CreateTagSetVersionUsecase),
		internalhandler.WithKnowledgeEventPort(container.SovereignClient),
		internalhandler.WithRAGToolPorts(container.FetchTagCloudUsecase, container.FetchArticlesByTagUsecase),
		internalhandler.WithRecapArticlesUsecase(container.RecapArticlesUsecase),
	)
	internalPath, internalServiceHandler := backendv1connect.NewBackendInternalServiceHandler(internalHandler, internalOpts)
	mux.Handle(internalPath, internalServiceHandler)
	logger.Info("Registered Connect-RPC BackendInternalService", "path", internalPath)
}

// CreateConnectServer creates the Connect-RPC server with HTTP/2 support.
func CreateConnectServer(container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Add health check endpoint for Connect-RPC server
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})

	SetupConnectHandlers(mux, container, cfg, logger)

	// Support HTTP/2 without TLS (h2c) for local development and internal communication
	return h2c.NewHandler(mux, &http2.Server{})
}
