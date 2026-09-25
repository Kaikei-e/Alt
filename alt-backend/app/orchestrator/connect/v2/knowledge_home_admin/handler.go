package knowledge_home_admin

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"alt/config"
	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/gen/proto/alt/knowledge_home/v1/knowledgehomev1connect"
	"alt/orchestrator/usecase/knowledge_backfill_usecase"
	"alt/orchestrator/usecase/knowledge_projection_health_usecase"
	"alt/orchestrator/usecase/knowledge_url_backfill_usecase"
	"alt/utils/safeconv"
)

// ReprojectUsecase defines the reproject operations interface.
type ReprojectUsecase interface {
	StartReproject(ctx context.Context, mode, fromVersion, toVersion string, rangeStart, rangeEnd *time.Time) (*domain.ReprojectRun, error)
	GetReprojectStatus(ctx context.Context, runID uuid.UUID) (*domain.ReprojectRun, error)
	ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]domain.ReprojectRun, error)
	CompareReproject(ctx context.Context, runID uuid.UUID) (*domain.ReprojectDiffSummary, error)
	SwapReproject(ctx context.Context, runID uuid.UUID) error
	RollbackReproject(ctx context.Context, runID uuid.UUID) error
}

// SLOUsecase defines the SLO status operations interface.
type SLOUsecase interface {
	GetSLOStatus(ctx context.Context) (*domain.SLOStatus, error)
}

// AuditUsecase defines the projection audit operations interface.
type AuditUsecase interface {
	RunProjectionAudit(ctx context.Context, projectionName, projectionVersion string, sampleSize int) (*domain.ProjectionAudit, error)
}

// MetricsUsecase defines the system metrics operations interface.
type MetricsUsecase interface {
	GetSystemMetrics(ctx context.Context) (*domain.SystemMetrics, error)
}

// URLBackfillUsecase emits ArticleUrlBackfilled corrective events for
// articles whose Knowledge Home projection has an empty URL. Defined
// as an interface so the handler can be unit-tested without booting
// the sovereign client; the production wiring passes a concrete
// *knowledge_url_backfill_usecase.Usecase.
type URLBackfillUsecase interface {
	Emit(ctx context.Context, maxArticles int, dryRun bool) (*knowledge_url_backfill_usecase.EmitResult, error)
}

// Handler implements KnowledgeHomeAdminServiceHandler.
type Handler struct {
	backfillUsecase         *knowledge_backfill_usecase.Usecase
	urlBackfillUsecase      URLBackfillUsecase
	projectionHealthUsecase *knowledge_projection_health_usecase.Usecase
	reprojectUsecase        ReprojectUsecase
	sloUsecase              SLOUsecase
	auditUsecase            AuditUsecase
	metricsUsecase          MetricsUsecase
	cfg                     *config.KnowledgeHomeConfig
	logger                  *slog.Logger
}

// Compile-time interface verification.
var _ knowledgehomev1connect.KnowledgeHomeAdminServiceHandler = (*Handler)(nil)

// NewHandler creates a new KnowledgeHomeAdminService handler.
func NewHandler(
	backfill *knowledge_backfill_usecase.Usecase,
	urlBackfill *knowledge_url_backfill_usecase.Usecase,
	health *knowledge_projection_health_usecase.Usecase,
	reproject ReprojectUsecase,
	slo SLOUsecase,
	audit AuditUsecase,
	metrics MetricsUsecase,
	cfg *config.KnowledgeHomeConfig,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		backfillUsecase:         backfill,
		urlBackfillUsecase:      urlBackfill,
		projectionHealthUsecase: health,
		reprojectUsecase:        reproject,
		sloUsecase:              slo,
		auditUsecase:            audit,
		metricsUsecase:          metrics,
		cfg:                     cfg,
		logger:                  logger,
	}
}

// GetFeatureFlags returns the current feature flag configuration.
func (h *Handler) GetFeatureFlags(
	_ context.Context,
	_ *connect.Request[knowledgehomev1.GetFeatureFlagsRequest],
) (*connect.Response[knowledgehomev1.GetFeatureFlagsResponse], error) {
	return connect.NewResponse(&knowledgehomev1.GetFeatureFlagsResponse{
		EnableHomePage:     h.cfg.EnableHomePage,
		EnableTracking:     h.cfg.EnableTracking,
		EnableProjectionV2: h.cfg.EnableProjectionV2,
		RolloutPercentage:  safeconv.Int32(h.cfg.RolloutPercentage),
		// EnableRecallRail intentionally absent — ADR-000913 §D-9
		// retired the flag once the recall rail merged into the
		// canonical Home payload. The proto field stays in the wire
		// shape for backward compatibility but reads as 0/false.
		EnableLens:          h.cfg.EnableLens,
		EnableStreamUpdates: h.cfg.EnableStreamUpdates,
		EnableSupersedeUx:   h.cfg.EnableSupersedeUX,
	}), nil
}
