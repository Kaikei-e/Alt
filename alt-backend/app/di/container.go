package di

import (
	"alt/orchestrator/driver/preprocessor_connect"
	"alt/orchestrator/gateway/feature_flag_gateway"
	"alt/orchestrator/gateway/fetch_article_gateway"
	"alt/orchestrator/gateway/preprocessor_summarize_gateway"
	"alt/orchestrator/gateway/rag_connect_gateway"
	"alt/orchestrator/port/morning_letter_port"
	"alt/orchestrator/port/rag_integration_port"
	"alt/orchestrator/usecase/answer_chat_usecase"
	"alt/orchestrator/usecase/archive_article_usecase"
	"alt/orchestrator/usecase/archive_lens_usecase"
	"alt/orchestrator/usecase/cached_feed_list_usecase"
	"alt/orchestrator/usecase/create_lens_usecase"
	"alt/orchestrator/usecase/csrf_token_usecase"
	dashboard_usecase "alt/orchestrator/usecase/dashboard"
	"alt/orchestrator/usecase/emit_trail_outcome_usecase"
	"alt/orchestrator/usecase/feed_link_usecase"
	"alt/orchestrator/usecase/fetch_article_summaries_usecase"
	"alt/orchestrator/usecase/fetch_article_summary_usecase"
	"alt/orchestrator/usecase/fetch_article_tags_usecase"
	"alt/orchestrator/usecase/fetch_article_usecase"
	"alt/orchestrator/usecase/fetch_articles_usecase"
	"alt/orchestrator/usecase/fetch_feed_details_usecase"
	"alt/orchestrator/usecase/fetch_feed_stats_usecase"
	"alt/orchestrator/usecase/fetch_feed_tags_by_id_usecase"
	"alt/orchestrator/usecase/fetch_feed_tags_usecase"
	"alt/orchestrator/usecase/fetch_feed_usecase"
	"alt/orchestrator/usecase/fetch_inoreader_summary_usecase"
	"alt/orchestrator/usecase/fetch_latest_article_usecase"
	"alt/orchestrator/usecase/fetch_random_subscription_usecase"
	"alt/orchestrator/usecase/fetch_trend_stats_usecase"
	"alt/orchestrator/usecase/get_article_source_url_usecase"
	"alt/orchestrator/usecase/get_item_branches_usecase"
	"alt/orchestrator/usecase/get_knowledge_home_usecase"
	"alt/orchestrator/usecase/get_knowledge_trail_usecase"
	"alt/orchestrator/usecase/image_fetch_usecase"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/orchestrator/usecase/knowledge_audit_usecase"
	"alt/orchestrator/usecase/knowledge_backfill_usecase"
	"alt/orchestrator/usecase/knowledge_metrics_usecase"
	"alt/orchestrator/usecase/knowledge_projection_health_usecase"
	"alt/orchestrator/usecase/knowledge_reproject_usecase"
	"alt/orchestrator/usecase/knowledge_slo_usecase"
	"alt/orchestrator/usecase/knowledge_url_backfill_usecase"
	"alt/orchestrator/usecase/list_lenses_usecase"
	"alt/orchestrator/usecase/og_image_resolve_usecase"
	"alt/orchestrator/usecase/opml_usecase"
	"alt/orchestrator/usecase/reading_status"
	"alt/orchestrator/usecase/recall_dismiss_usecase"
	"alt/orchestrator/usecase/recall_rail_usecase"
	"alt/orchestrator/usecase/recall_snooze_usecase"
	"alt/orchestrator/usecase/recap_usecase"
	"alt/orchestrator/usecase/register_favorite_feed_usecase"
	"alt/orchestrator/usecase/register_feed_usecase"
	"alt/orchestrator/usecase/remove_favorite_feed_usecase"
	"alt/orchestrator/usecase/resolve_trail_branch_usecase"
	"alt/orchestrator/usecase/retrieve_context_usecase"
	"alt/orchestrator/usecase/scraping_domain_usecase"
	"alt/orchestrator/usecase/search_article_usecase"
	"alt/orchestrator/usecase/search_feed_usecase"
	"alt/orchestrator/usecase/search_trail_usecase"
	"alt/orchestrator/usecase/select_lens_usecase"
	"alt/orchestrator/usecase/stream_article_tags_usecase"
	"alt/orchestrator/usecase/subscription_usecase"
	"alt/orchestrator/usecase/summarize_article_usecase"
	"alt/orchestrator/usecase/track_home_action_usecase"
	"alt/orchestrator/usecase/track_home_seen_usecase"
	"alt/orchestrator/usecase/update_lens_usecase"
	"alt/shared/driver/sovereign_client"
	"alt/shared/usecase/create_summary_version_usecase"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/shared/usecase/fetch_tag_cloud_usecase"
	"alt/utils/batch_article_fetcher"
	altotel "alt/utils/otel"
)

// ApplicationComponents is cmd/backend's component set — see
// container_backend.go for the composition root that builds it, and
// container_harvester.go / di/datahub for the other two binaries.
//
// Fields absent here are absent on purpose: what the backend does not build
// must not be reachable from a backend handler even as a nil value. See
// NewBackendComponents for the list and the reasoning.
type ApplicationComponents struct {
	// Domain modules
	Infra        *InfraModule
	Feed         *FeedModule
	Article      *ArticleModule
	Knowledge    *KnowledgeModule
	RAG          *RAGModule
	Image        *ImageModule
	Recap        *RecapModule
	Subscription *SubscriptionModule

	// ===== BACKWARD COMPAT: All existing fields populated from modules =====
	// These allow existing handler code to continue working unchanged.
	// They will be removed in a future phase when handlers access modules directly.

	// There is no AltDBRepository field and no InternalArticleGateway. Both
	// went with the database pool in ADR-000954 Wave 3 batch 6: cmd/backend
	// reaches alt_db only through alt-data-hub now, and a handler that reaches
	// for a repository here fails to compile rather than dereferencing a nil.

	// Ports
	RagIntegration   rag_integration_port.RagIntegrationPort
	RagConnectClient *rag_connect_gateway.Client
	StreamChatPort   morning_letter_port.StreamChatPort

	// Usecases
	FetchSingleFeedUsecase              *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase               *fetch_feed_usecase.FetchFeedsListUsecase
	FetchFeedsListCursorUsecase         *fetch_feed_usecase.FetchFeedsListCursorUsecase
	FetchUnreadFeedsListCursorUsecase   *fetch_feed_usecase.FetchUnreadFeedsListCursorUsecase
	CachedFeedListUsecase               *cached_feed_list_usecase.CachedFeedListUsecase
	FetchReadFeedsListCursorUsecase     *fetch_feed_usecase.FetchReadFeedsListCursorUsecase
	FetchFavoriteFeedsListCursorUsecase *fetch_feed_usecase.FetchFavoriteFeedsListCursorUsecase
	RegisterFeedsUsecase                *register_feed_usecase.RegisterFeedsUsecase
	RegisterFavoriteFeedUsecase         *register_favorite_feed_usecase.RegisterFavoriteFeedUsecase
	RemoveFavoriteFeedUsecase           *remove_favorite_feed_usecase.RemoveFavoriteFeedUsecase
	ListFeedLinksUsecase                *feed_link_usecase.ListFeedLinksUsecase
	ListFeedLinksWithHealthUsecase      *feed_link_usecase.ListFeedLinksWithHealthUsecase
	DeleteFeedLinkUsecase               *feed_link_usecase.DeleteFeedLinkUsecase
	FeedsReadingStatusUsecase           *reading_status.FeedsReadingStatusUsecase
	ArticlesReadingStatusUsecase        *reading_status.ArticlesReadingStatusUsecase
	FeedsSummaryUsecase                 *fetch_feed_details_usecase.FeedsSummaryUsecase
	FeedAmountUsecase                   *fetch_feed_stats_usecase.FeedsCountUsecase
	UnsummarizedArticlesCountUsecase    *fetch_feed_stats_usecase.UnsummarizedArticlesCountUsecase
	SummarizedArticlesCountUsecase      *fetch_feed_stats_usecase.SummarizedArticlesCountUsecase
	TotalArticlesCountUsecase           *fetch_feed_stats_usecase.TotalArticlesCountUsecase
	TodayUnreadArticlesCountUsecase     *fetch_feed_stats_usecase.TodayUnreadArticlesCountUsecase
	TrendStatsUsecase                   *fetch_trend_stats_usecase.FetchTrendStatsUsecase
	FeedSearchUsecase                   *search_feed_usecase.SearchFeedMeilisearchUsecase
	ArticleSearchUsecase                *search_article_usecase.SearchArticleUsecase
	FetchFeedTagsUsecase                *fetch_feed_tags_usecase.FetchFeedTagsUsecase
	FetchFeedTagsByIDUsecase            *fetch_feed_tags_by_id_usecase.FetchFeedTagsByIDUsecase
	FetchInoreaderSummaryUsecase        fetch_inoreader_summary_usecase.FetchInoreaderSummaryUsecase
	ImageFetchUsecase                   image_fetch_usecase.ImageFetchUsecaseInterface
	CSRFTokenUsecase                    *csrf_token_usecase.CSRFTokenUsecase
	ArticleUsecase                      fetch_article_usecase.ArticleUsecase
	ArchiveArticleUsecase               *archive_article_usecase.ArchiveArticleUsecase
	FetchArticlesCursorUsecase          *fetch_articles_usecase.FetchArticlesCursorUsecase
	RecapUsecase                        *recap_usecase.RecapUsecase
	MorningUsecase                      morning_letter_port.MorningUsecase
	MorningLetterUsecase                morning_letter_port.MorningLetterUsecase
	ScrapingDomainUsecase               *scraping_domain_usecase.ScrapingDomainUsecase
	BatchArticleFetcher                 *batch_article_fetcher.BatchArticleFetcher
	FetchArticleGateway                 *fetch_article_gateway.FetchArticleGateway
	RetrieveContextUsecase              retrieve_context_usecase.RetrieveContextUsecase
	AnswerChatUsecase                   answer_chat_usecase.AnswerChatUsecase
	FetchRandomSubscriptionUsecase      *fetch_random_subscription_usecase.FetchRandomSubscriptionUsecase
	FetchArticlesByTagUsecase           *fetch_articles_by_tag_usecase.FetchArticlesByTagUsecase
	FetchArticleTagsUsecase             *fetch_article_tags_usecase.FetchArticleTagsUsecase
	GetRecapJobsUsecase                 dashboard_usecase.GetRecapJobsUsecase
	ListSubscriptionsUsecase            *subscription_usecase.ListSubscriptionsUsecase
	SubscribeUsecase                    *subscription_usecase.SubscribeUsecase
	UnsubscribeUsecase                  *subscription_usecase.UnsubscribeUsecase

	DashboardMetricsUsecase    *dashboard_usecase.DashboardMetricsUsecase
	FetchLatestArticleUsecase  *fetch_latest_article_usecase.FetchLatestArticleUsecase
	FetchArticleSummaryUsecase *fetch_article_summary_usecase.FetchArticleSummaryUsecase
	StreamArticleTagsUsecase   *stream_article_tags_usecase.StreamArticleTagsUsecase
	FetchTagCloudUsecase       *fetch_tag_cloud_usecase.FetchTagCloudUsecase
	GetArticleSourceURLUsecase *get_article_source_url_usecase.GetArticleSourceURLUsecase

	// Legacy REST v1 summarize endpoints (POST /v1/feeds/summarize,
	// /summarize/queue, GET /summarize/status/:job_id, POST /fetch/summary)
	SummarizeArticleUsecase      *summarize_article_usecase.Usecase
	FetchArticleSummariesUsecase *fetch_article_summaries_usecase.Usecase
	PreProcessorSummarizeGateway *preprocessor_summarize_gateway.Gateway

	// OPML
	ExportOPMLUsecase *opml_usecase.ExportOPMLUsecase
	ImportOPMLUsecase *opml_usecase.ImportOPMLUsecase

	// Image Proxy
	ImageProxyUsecase *image_proxy_usecase.ImageProxyUsecase
	// ResolveOgImagesUsecase resolves og:image URLs for feeds a reader has
	// brought into view. Nil exactly when the image proxy is not wired.
	ResolveOgImagesUsecase *og_image_resolve_usecase.Usecase

	// Service-to-service Connect-RPC clients
	PreProcessorConnectClient *preprocessor_connect.ConnectPreProcessorClient

	// Knowledge Home
	GetKnowledgeHomeUsecase          *get_knowledge_home_usecase.GetKnowledgeHomeUsecase
	GetKnowledgeTrailUsecase         *get_knowledge_trail_usecase.GetKnowledgeTrailUsecase
	ResolveTrailBranchUsecase        *resolve_trail_branch_usecase.ResolveTrailBranchUsecase
	EmitTrailOutcomeUsecase          *emit_trail_outcome_usecase.EmitTrailOutcomeUsecase
	SearchTrailUsecase               *search_trail_usecase.SearchTrailUsecase
	GetItemBranchesUsecase           *get_item_branches_usecase.GetItemBranchesUsecase
	TrackHomeSeenUsecase             *track_home_seen_usecase.TrackHomeSeenUsecase
	TrackHomeActionUsecase           *track_home_action_usecase.TrackHomeActionUsecase
	CreateSummaryVersionUsecase      *create_summary_version_usecase.CreateSummaryVersionUsecase
	FeatureFlagGateway               *feature_flag_gateway.Gateway
	KnowledgeBackfillUsecase         *knowledge_backfill_usecase.Usecase
	KnowledgeURLBackfillUsecase      *knowledge_url_backfill_usecase.Usecase
	KnowledgeProjectionHealthUsecase *knowledge_projection_health_usecase.Usecase
	ReprojectUsecase                 *knowledge_reproject_usecase.Usecase
	SLOUsecase                       *knowledge_slo_usecase.Usecase
	AuditUsecase                     *knowledge_audit_usecase.Usecase
	MetricsUsecase                   *knowledge_metrics_usecase.Usecase

	// Phase 4: RecallRail, Lens, Stream, Supersede
	RecallRailUsecase    *recall_rail_usecase.RecallRailUsecase
	RecallSnoozeUsecase  *recall_snooze_usecase.RecallSnoozeUsecase
	RecallDismissUsecase *recall_dismiss_usecase.RecallDismissUsecase
	CreateLensUsecase    *create_lens_usecase.CreateLensUsecase
	UpdateLensUsecase    *update_lens_usecase.UpdateLensUsecase
	ListLensesUsecase    *list_lenses_usecase.ListLensesUsecase
	SelectLensUsecase    *select_lens_usecase.SelectLensUsecase
	ArchiveLensUsecase   *archive_lens_usecase.ArchiveLensUsecase

	// Knowledge Sovereign (remote Connect-RPC service — all knowledge data access)
	SovereignClient *sovereign_client.Client

	// Search
	Search *SearchModule

	// Observability
	KnowledgeHomeMetrics *altotel.KnowledgeHomeMetrics

	// Admin observability (Prometheus-backed metrics UI). Facade may be nil
	// when cfg.AdminMonitor.Enabled is false; server.go skips registration.
	AdminMonitor *AdminMonitorModule
}
