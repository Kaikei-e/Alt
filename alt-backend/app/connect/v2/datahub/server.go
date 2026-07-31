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
	"time"

	"connectrpc.com/connect"

	"alt/config"
	"alt/connect/v2/middleware"
	"alt/connect/v2/muxutil"
	"alt/dataplane/connect/datahubapi"
	internalhandler "alt/dataplane/connect/internalapi"
	"alt/di"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/gen/proto/services/backend/v1/backendv1connect"
)

// SetupConnectHandlers registers the service-to-service API cmd/datahub serves
// behind mutual TLS — currently under two names.
//
// alt.datahub.v1.DataHubService is the contract going forward (ADR-000954 D7).
// services.backend.v1.BackendInternalService is the same 24 procedures under
// the name they had while this code lived inside alt-backend, kept mounted
// until Wave 2-B has moved all seven peers. Both paths reach one
// implementation: the DataHubService handler is an adapter over the internal
// one, so there is no second copy of the transaction boundaries to keep in
// step.
//
// It takes *di.DataHubComponents rather than the backend's component set: the
// event publisher, the Kratos client and the recap/tag-set read models it
// needs are built by that binary alone, so a backend handler cannot reach them
// even by accident (CLAUDE.md rule 8 — absent field, compile error).
func SetupConnectHandlers(mux *http.ServeMux, container *di.DataHubComponents, cfg *config.Config, logger *slog.Logger) {
	cancelInterceptor := middleware.NewContextCancelInterceptor(logger)

	legacyNotice := newLegacyNamespaceNotice(logger, legacyNamespaceLogInterval, time.Now)

	internalOpts := connect.WithInterceptors(
		cancelInterceptor.Interceptor(),
		legacyNotice.interceptor(),
	)
	datahubOpts := connect.WithInterceptors(
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

	datahubHandler := datahubapi.NewHandler(
		internalHandler,
		container.KratosClient,
		container.FetchRecentArticlesUsecase,
		logger,
	)
	datahubPath, datahubServiceHandler := datahubv1connect.NewDataHubServiceHandler(datahubHandler, datahubOpts)
	mux.Handle(datahubPath, datahubServiceHandler)

	internalPath, internalServiceHandler := backendv1connect.NewBackendInternalServiceHandler(internalHandler, internalOpts)
	mux.Handle(internalPath, internalServiceHandler)

	// One line at startup naming both mounts, so "which namespaces is this
	// process answering on" is answerable from the boot log rather than only
	// from whichever peer happens to call during the next five minutes
	// (CLAUDE.md rule 8). The per-request half lives in deprecation.go.
	logger.Warn("datahub_namespaces.wiring",
		"current", datahubPath,
		"deprecated", internalPath,
		"deprecated_removed_in", "ADR-000954 Wave 2-C",
		"note", "both paths serve one implementation; the deprecated mount is an adapter target, not a copy",
	)
}

// CreateServer builds the Connect-RPC handler for data-hub's mutual-TLS
// listener: BackendInternalService and /health, and nothing else.
func CreateServer(container *di.DataHubComponents, cfg *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	muxutil.RegisterHealth(mux)
	SetupConnectHandlers(mux, container, cfg, logger)

	return muxutil.WithH2C(mux)
}
