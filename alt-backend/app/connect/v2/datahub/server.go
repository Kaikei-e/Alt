// Package datahub provides the Connect-RPC server for cmd/datahub's
// mutual-TLS listener.
//
// It is a package of its own rather than another function in alt/connect/v2 so
// that the two surfaces do not link each other. cmd/backend must not contain
// BackendInternalService's handler at all — not merely leave it unmounted —
// and cmd/datahub must not contain the browser-facing handlers. Sharing a
// package would compile both into both.
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
	internalhandler "alt/dataplane/connect/internalapi"
	"alt/di"
	"alt/gen/proto/services/backend/v1/backendv1connect"
)

// SetupDataHubConnectHandlers registers BackendInternalService, the
// service-to-service API cmd/datahub serves behind mutual TLS.
//
// It takes *di.DataHubComponents rather than the backend's component set: the
// event publisher, the Kratos client and the recap/tag-set read models it
// needs are built by that binary alone, so a backend handler cannot reach them
// even by accident (CLAUDE.md rule 8 — absent field, compile error).
func SetupConnectHandlers(mux *http.ServeMux, container *di.DataHubComponents, cfg *config.Config, logger *slog.Logger) {
	cancelInterceptor := middleware.NewContextCancelInterceptor(logger)

	internalOpts := connect.WithInterceptors(
		cancelInterceptor.Interceptor(),
	)
	gw := container.InternalArticleGateway
	internalHandler := internalhandler.NewHandler(
		gw, gw, gw, gw, gw,
		logger,
		internalhandler.WithPhase2Ports(gw, gw, gw, gw, gw, gw),
		internalhandler.WithPhase3Ports(gw, gw, gw),
		internalhandler.WithBatchGetTagsPort(gw),
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

// CreateServer builds the Connect-RPC handler for data-hub's mutual-TLS
// listener: BackendInternalService and /health, and nothing else.
func CreateServer(container *di.DataHubComponents, cfg *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	muxutil.RegisterHealth(mux)
	SetupConnectHandlers(mux, container, cfg, logger)

	return muxutil.WithH2C(mux)
}
