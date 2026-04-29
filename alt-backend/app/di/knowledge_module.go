package di

import (
	"alt/driver/health_checker"
	"alt/driver/sovereign_client"
	"alt/gateway/feature_flag_gateway"
	"alt/gateway/knowledge_backfill_gateway"
	"alt/gateway/knowledge_metrics_gateway"
	"alt/gateway/summary_version_gateway"
	"alt/gateway/tag_set_version_gateway"
	"alt/gateway/trending_tags_gateway"
	"alt/usecase/append_knowledge_event_usecase"
	"alt/usecase/archive_lens_usecase"
	"alt/usecase/create_lens_usecase"
	"alt/usecase/create_summary_version_usecase"
	"alt/usecase/create_tag_set_version_usecase"
	"alt/usecase/get_knowledge_home_usecase"
	"alt/usecase/knowledge_audit_usecase"
	"alt/usecase/knowledge_backfill_usecase"
	"alt/usecase/knowledge_loop_usecase"
	"alt/usecase/knowledge_metrics_usecase"
	"alt/usecase/knowledge_projection_health_usecase"
	"alt/usecase/knowledge_reproject_usecase"
	"alt/usecase/knowledge_slo_usecase"
	"alt/usecase/knowledge_url_backfill_usecase"
	"alt/usecase/list_lenses_usecase"
	"alt/usecase/recall_dismiss_usecase"
	"alt/usecase/recall_rail_usecase"
	"alt/usecase/recall_snooze_usecase"
	"alt/usecase/select_lens_usecase"
	"alt/usecase/track_home_action_usecase"
	"alt/usecase/track_home_seen_usecase"
	"alt/usecase/update_lens_usecase"
	altotel "alt/utils/otel"
	"log/slog"
	"os"
	"time"
)

// KnowledgeModule holds all Knowledge Home domain components.
type KnowledgeModule struct {
	// Usecases
	GetKnowledgeHomeUsecase          *get_knowledge_home_usecase.GetKnowledgeHomeUsecase
	TrackHomeSeenUsecase             *track_home_seen_usecase.TrackHomeSeenUsecase
	TrackHomeActionUsecase           *track_home_action_usecase.TrackHomeActionUsecase
	AppendKnowledgeEventUsecase      *append_knowledge_event_usecase.AppendKnowledgeEventUsecase
	CreateSummaryVersionUsecase      *create_summary_version_usecase.CreateSummaryVersionUsecase
	CreateTagSetVersionUsecase       *create_tag_set_version_usecase.CreateTagSetVersionUsecase
	KnowledgeBackfillUsecase         *knowledge_backfill_usecase.Usecase
	KnowledgeURLBackfillUsecase      *knowledge_url_backfill_usecase.Usecase
	KnowledgeProjectionHealthUsecase *knowledge_projection_health_usecase.Usecase
	ReprojectUsecase                 *knowledge_reproject_usecase.Usecase
	SLOUsecase                       *knowledge_slo_usecase.Usecase
	AuditUsecase                     *knowledge_audit_usecase.Usecase
	MetricsUsecase                   *knowledge_metrics_usecase.Usecase

	// Recall / Lens usecases
	RecallRailUsecase    *recall_rail_usecase.RecallRailUsecase
	RecallSnoozeUsecase  *recall_snooze_usecase.RecallSnoozeUsecase
	RecallDismissUsecase *recall_dismiss_usecase.RecallDismissUsecase
	CreateLensUsecase    *create_lens_usecase.CreateLensUsecase
	UpdateLensUsecase    *update_lens_usecase.UpdateLensUsecase
	ListLensesUsecase    *list_lenses_usecase.ListLensesUsecase
	SelectLensUsecase    *select_lens_usecase.SelectLensUsecase
	ArchiveLensUsecase   *archive_lens_usecase.ArchiveLensUsecase

	// Knowledge Loop usecases (new projection; see docs/ADR/000831.md).
	// Storage is sovereign-owned: the usecase talks to sovereign_client.Client which
	// implements all Knowledge Loop ports; alt-db has no Knowledge Loop tables.
	GetKnowledgeLoopUsecase        *knowledge_loop_usecase.GetKnowledgeLoopUsecase
	TransitionKnowledgeLoopUsecase *knowledge_loop_usecase.TransitionKnowledgeLoopUsecase

	// Gateways
	FeatureFlagGateway               *feature_flag_gateway.Gateway
	KnowledgeBackfillArticlesGateway *knowledge_backfill_gateway.Gateway
	SummaryVersionGateway            *summary_version_gateway.Gateway
	TagSetVersionGateway             *tag_set_version_gateway.Gateway

	// Sovereign client
	SovereignClient *sovereign_client.Client

	// Observability
	KnowledgeHomeMetrics *altotel.KnowledgeHomeMetrics
}

func newKnowledgeModule(infra *InfraModule, article *ArticleModule) *KnowledgeModule {
	altDB := infra.AltDBRepository
	cfg := infra.Config

	// Knowledge Sovereign: all knowledge data access via Connect-RPC
	sovereignURL := os.Getenv("SOVEREIGN_URL")
	sovereignEnabled := sovereignURL != ""
	sovereignCli := sovereign_client.NewClient(sovereignURL, sovereignEnabled)

	// Knowledge Home gateways
	summaryVersionGw := summary_version_gateway.NewGateway(altDB)
	tagSetVersionGw := tag_set_version_gateway.NewGateway(altDB)
	featureFlagGw := feature_flag_gateway.NewGateway(&cfg.KnowledgeHome)
	knowledgeBackfillGw := knowledge_backfill_gateway.NewGateway(altDB)

	// Knowledge Home usecases
	trendingTagsGw := trending_tags_gateway.NewTrendingTagsGateway(altDB, 30*time.Minute)
	getKnowledgeHomeUC := get_knowledge_home_usecase.NewGetKnowledgeHomeUsecase(sovereignCli, sovereignCli, sovereignCli, sovereignCli, sovereignCli, trendingTagsGw)
	trackHomeSeenUC := track_home_seen_usecase.NewTrackHomeSeenUsecase(sovereignCli, featureFlagGw)
	trackHomeActionUC := track_home_action_usecase.NewTrackHomeActionUsecase(sovereignCli, sovereignCli, featureFlagGw, sovereignCli, sovereignCli, sovereignCli)
	appendKnowledgeEventUC := append_knowledge_event_usecase.NewAppendKnowledgeEventUsecase(sovereignCli)
	createSummaryVersionUC := create_summary_version_usecase.NewCreateSummaryVersionUsecase(summaryVersionGw, sovereignCli, summaryVersionGw)
	createTagSetVersionUC := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetVersionGw, sovereignCli, tagSetVersionGw)
	knowledgeBackfillUC := knowledge_backfill_usecase.NewUsecase(
		sovereignCli,
		sovereignCli,
		sovereignCli,
		sovereignCli,
		knowledgeBackfillGw, // ListBackfillArticlesPort (articles table in alt-db)
		sovereignCli,
	)
	knowledgeURLBackfillUC := knowledge_url_backfill_usecase.NewUsecase(
		knowledgeBackfillGw, // same articles source as TriggerBackfill
		sovereignCli,        // AppendKnowledgeEventPort
	)
	knowledgeProjectionHealthUC := knowledge_projection_health_usecase.NewUsecase(sovereignCli, sovereignCli, sovereignCli, sovereignCli)

	// Reproject, SLO, Audit
	reprojectUC := knowledge_reproject_usecase.NewUsecase(
		sovereignCli,
		sovereignCli,
		sovereignCli,
		sovereignCli,
		sovereignCli,
		sovereignCli,
		sovereignCli,
		sovereignCli,
	).WithUpdateCheckpointPort(sovereignCli)
	sloUC := knowledge_slo_usecase.NewUsecase(sovereignCli)
	auditUC := knowledge_audit_usecase.NewUsecase(sovereignCli, sovereignCli)

	// System metrics: health check endpoints with sensible defaults
	sovereignMetricsURL := os.Getenv("SOVEREIGN_METRICS_URL")
	if sovereignMetricsURL == "" {
		sovereignMetricsURL = "http://knowledge-sovereign:9501"
	}
	meiliURL := os.Getenv("MEILISEARCH_HOST")
	if meiliURL == "" {
		meiliURL = "http://meilisearch:7700"
	}
	healthEndpoints := []health_checker.ServiceEndpoint{
		{Name: "knowledge-sovereign", Endpoint: sovereignMetricsURL + "/health"},
		{Name: "meilisearch", Endpoint: meiliURL + "/health"},
	}
	healthChecker := health_checker.NewChecker(healthEndpoints)

	// RecallRail, Lens, Supersede
	recallRailUC := recall_rail_usecase.NewRecallRailUsecase(sovereignCli, featureFlagGw, article.InternalArticleGateway)
	recallSnoozeUC := recall_snooze_usecase.NewRecallSnoozeUsecase(sovereignCli, sovereignCli)
	recallDismissUC := recall_dismiss_usecase.NewRecallDismissUsecase(sovereignCli, sovereignCli)
	createLensUC := create_lens_usecase.NewCreateLensUsecase(sovereignCli, sovereignCli)
	updateLensUC := update_lens_usecase.NewUpdateLensUsecase(sovereignCli, sovereignCli)
	listLensesUC := list_lenses_usecase.NewListLensesUsecase(sovereignCli, sovereignCli)
	selectLensUC := select_lens_usecase.NewSelectLensUsecase(sovereignCli, sovereignCli, sovereignCli, sovereignCli)
	archiveLensUC := archive_lens_usecase.NewArchiveLensUsecase(sovereignCli, sovereignCli)

	// Knowledge Home metrics (optional, fail-open)
	var knowledgeHomeMetrics *altotel.KnowledgeHomeMetrics
	if m, err := altotel.NewKnowledgeHomeMetrics(); err != nil {
		slog.Warn("failed to initialize KnowledgeHomeMetrics, continuing without metrics", "error", err)
	} else {
		knowledgeHomeMetrics = m
	}

	// Wire metrics usecase (uses metrics snapshot from KnowledgeHomeMetrics)
	var metricsSnapshot *altotel.MetricsSnapshot
	if knowledgeHomeMetrics != nil {
		metricsSnapshot = knowledgeHomeMetrics.Snapshot
	}
	metricsGw := knowledge_metrics_gateway.NewGateway(metricsSnapshot)
	metricsUC := knowledge_metrics_usecase.NewUsecase(metricsGw, healthChecker)

	// Knowledge Loop wiring: storage lives in knowledge-sovereign, not alt-db.
	// The sovereign_client.Client implements all Knowledge Loop ports (read + write + dedupe).
	getKnowledgeLoopUC := knowledge_loop_usecase.NewGetKnowledgeLoopUsecase(
		sovereignCli,
		sovereignCli,
		sovereignCli,
	)
	// sovereignCli implements both ReserveTransitionIdempotencyPort and AppendKnowledgeEventPort,
	// so the transition usecase reserves idempotency and appends the Loop event through the
	// same sovereign Connect-RPC client (single source of truth for knowledge_events).
	//
	// The rate limiter is shared process-wide so the canonical contract §8.4 minute
	// ceiling (600 Loop events/user/minute + 60s per-entry dwell throttle) holds
	// across concurrent /loop/transition requests. One process == one bucket set;
	// a multi-pod deployment drifts by N× pods on the ceiling, which we accept:
	// idempotency + dedupe guard prevent event store damage, and this limiter is
	// the defense-in-depth layer on top.
	knowledgeLoopRateLimiter := knowledge_loop_usecase.NewLoopRateLimiter(nil)
	transitionKnowledgeLoopUC := knowledge_loop_usecase.NewTransitionKnowledgeLoopUsecase(
		sovereignCli,
		sovereignCli,
		knowledgeLoopRateLimiter,
		nil, // use time.Now by default
	)

	return &KnowledgeModule{
		GetKnowledgeHomeUsecase:          getKnowledgeHomeUC,
		TrackHomeSeenUsecase:             trackHomeSeenUC,
		TrackHomeActionUsecase:           trackHomeActionUC,
		AppendKnowledgeEventUsecase:      appendKnowledgeEventUC,
		CreateSummaryVersionUsecase:      createSummaryVersionUC,
		CreateTagSetVersionUsecase:       createTagSetVersionUC,
		KnowledgeBackfillUsecase:         knowledgeBackfillUC,
		KnowledgeURLBackfillUsecase:      knowledgeURLBackfillUC,
		KnowledgeProjectionHealthUsecase: knowledgeProjectionHealthUC,
		ReprojectUsecase:                 reprojectUC,
		SLOUsecase:                       sloUC,
		AuditUsecase:                     auditUC,
		MetricsUsecase:                   metricsUC,

		RecallRailUsecase:    recallRailUC,
		RecallSnoozeUsecase:  recallSnoozeUC,
		RecallDismissUsecase: recallDismissUC,
		CreateLensUsecase:    createLensUC,
		UpdateLensUsecase:    updateLensUC,
		ListLensesUsecase:    listLensesUC,
		SelectLensUsecase:    selectLensUC,
		ArchiveLensUsecase:   archiveLensUC,

		GetKnowledgeLoopUsecase:        getKnowledgeLoopUC,
		TransitionKnowledgeLoopUsecase: transitionKnowledgeLoopUC,

		FeatureFlagGateway:               featureFlagGw,
		KnowledgeBackfillArticlesGateway: knowledgeBackfillGw,
		SummaryVersionGateway:            summaryVersionGw,
		TagSetVersionGateway:             tagSetVersionGw,

		SovereignClient: sovereignCli,

		KnowledgeHomeMetrics: knowledgeHomeMetrics,
	}
}
