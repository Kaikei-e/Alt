// Package v2 provides Connect-RPC server setup and configuration.
package v2

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"alt/gen/proto/alt/articles/v2/articlesv2connect"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
	"alt/gen/proto/alt/recap/v2/recapv2connect"
	"alt/gen/proto/alt/rss/v2/rssv2connect"
	"alt/gen/proto/services/backend/v1/backendv1connect"

	"alt/config"
	"alt/connect/v2/articles"
	"alt/connect/v2/augur"
	"alt/connect/v2/feeds"
	internalhandler "alt/connect/v2/internal"
	"alt/connect/v2/middleware"
	"alt/connect/v2/morning_letter"
	"alt/connect/v2/recap"
	"alt/connect/v2/rss"
	"alt/di"
	recapinternal "alt/internal/recap"
)

// SetupConnectHandlers registers all Connect-RPC handlers with the HTTP mux.
func SetupConnectHandlers(mux *http.ServeMux, container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) {
	// Create auth interceptor
	authInterceptor := middleware.NewAuthInterceptor(logger, cfg)

	// Handler options with auth interceptor
	opts := connect.WithInterceptors(authInterceptor.Interceptor())

	// Register Feed service
	feedHandler := feeds.NewHandler(container, cfg, logger)
	feedPath, feedServiceHandler := feedsv2connect.NewFeedServiceHandler(feedHandler, opts)
	mux.Handle(feedPath, feedServiceHandler)
	logger.Info("Registered Connect-RPC FeedService", "path", feedPath)

	// Register Article service
	articleHandler := articles.NewHandler(container, cfg, logger)
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

	// Register MorningLetter service
	morningLetterHandler := morning_letter.NewHandler(container.MorningLetterConnectGateway, logger)
	morningLetterPath, morningLetterServiceHandler := morningletterv2connect.NewMorningLetterServiceHandler(morningLetterHandler, opts)
	mux.Handle(morningLetterPath, morningLetterServiceHandler)
	logger.Info("Registered Connect-RPC MorningLetterService", "path", morningLetterPath)

	// Register Recap service
	clusterDraftLoader := recapinternal.NewClusterDraftLoader(cfg.Recap.ClusterDraftPath)
	recapHandler := recap.NewHandler(container.RecapUsecase, clusterDraftLoader, logger)
	recapPath, recapServiceHandler := recapv2connect.NewRecapServiceHandler(recapHandler, opts)
	mux.Handle(recapPath, recapServiceHandler)
	logger.Info("Registered Connect-RPC RecapService", "path", recapPath)

	// Register BackendInternalService (service-to-service API, uses service token auth)
	serviceAuthInterceptor := middleware.NewServiceAuthInterceptor(logger, cfg.InternalAPI.ServiceSecret)
	internalOpts := connect.WithInterceptors(serviceAuthInterceptor.Interceptor())
	gw := container.InternalArticleGateway
	internalHandler := internalhandler.NewHandler(
		gw, gw, gw, gw, gw,
		logger,
		internalhandler.WithPhase2Ports(gw, gw, gw, gw, gw, gw),
		internalhandler.WithPhase3Ports(gw, gw, gw),
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
