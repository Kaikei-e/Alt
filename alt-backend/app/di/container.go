package di

import (
	"alt/driver/kratos_client"
	"alt/driver/preprocessor_connect"
	"alt/driver/sovereign_client"
	"alt/gateway/feature_flag_gateway"
	"alt/gateway/internal_article_gateway"
	"alt/gateway/knowledge_backfill_gateway"
	"alt/gateway/rag_connect_gateway"
	"alt/gateway/summary_version_gateway"
	"alt/gateway/tag_set_version_gateway"
	"alt/port/config_port"
	"alt/port/error_handler_port"
	"alt/port/event_publisher_port"
	"alt/port/morning_letter_port"
	"alt/port/rag_integration_port"
	"alt/port/rate_limiter_port"
	"alt/usecase/answer_chat_usecase"
	"alt/usecase/append_knowledge_event_usecase"
	"alt/usecase/archive_article_usecase"
	"alt/usecase/archive_lens_usecase"
	"alt/usecase/cached_feed_list_usecase"
	"alt/usecase/create_lens_usecase"
	"alt/usecase/create_summary_version_usecase"
	"alt/usecase/create_tag_set_version_usecase"
	"alt/usecase/csrf_token_usecase"
	dashboard_usecase "alt/usecase/dashboard"
	"alt/usecase/feed_link_usecase"
	"alt/usecase/fetch_article_summary_usecase"
	"alt/usecase/fetch_article_tags_usecase"
	"alt/usecase/fetch_article_usecase"
	"alt/usecase/fetch_articles_by_tag_usecase"
	"alt/usecase/fetch_articles_usecase"
	"alt/usecase/fetch_feed_details_usecase"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/fetch_feed_tags_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/fetch_inoreader_summary_usecase"
	"alt/usecase/fetch_latest_article_usecase"
	"alt/usecase/fetch_random_subscription_usecase"
	"alt/usecase/fetch_recent_articles_usecase"
	"alt/usecase/fetch_tag_cloud_usecase"
	"alt/usecase/fetch_trend_stats_usecase"
	"alt/usecase/get_knowledge_home_usecase"
	"alt/usecase/image_fetch_usecase"
	"alt/usecase/image_proxy_usecase"
	"alt/usecase/knowledge_audit_usecase"
	"alt/usecase/knowledge_backfill_usecase"
	"alt/usecase/knowledge_loop_usecase"
	"alt/usecase/knowledge_metrics_usecase"
	"alt/usecase/knowledge_projection_health_usecase"
	"alt/usecase/knowledge_reproject_usecase"
	"alt/usecase/knowledge_slo_usecase"
	"alt/usecase/list_lenses_usecase"
	"alt/usecase/opml_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/recall_dismiss_usecase"
	"alt/usecase/recall_rail_usecase"
	"alt/usecase/recall_snooze_usecase"
	"alt/usecase/recap_articles_usecase"
	"alt/usecase/recap_usecase"
	"alt/usecase/register_favorite_feed_usecase"
	"alt/usecase/register_feed_usecase"
	"alt/usecase/remove_favorite_feed_usecase"
	"alt/usecase/retrieve_context_usecase"
	"alt/usecase/scraping_domain_usecase"
	"alt/usecase/search_article_usecase"
	"alt/usecase/search_feed_usecase"
	"alt/usecase/select_lens_usecase"
	"alt/usecase/stream_article_tags_usecase"
	"alt/usecase/subscription_usecase"
	"alt/usecase/track_home_action_usecase"
	"alt/usecase/track_home_seen_usecase"
	"alt/usecase/update_lens_usecase"
	"alt/utils/batch_article_fetcher"
	altotel "alt/utils/otel"
	"log/slog"

	"alt/driver/alt_db"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

	// Ports
	ConfigPort       config_port.ConfigPort
	RateLimiterPort  rate_limiter_port.RateLimiterPort
	ErrorHandlerPort error_handler_port.ErrorHandlerPort

	// Repository
	AltDBRepository *alt_db.AltDBRepository

	// Drivers
	KratosClient kratos_client.KratosClient

	// Ports
	RagIntegration   rag_integration_port.RagIntegrationPort
	RagConnectClient *rag_connect_gateway.Client
	StreamChatPort   morning_letter_port.StreamChatPort
	EventPublisher   event_publisher_port.EventPublisherPort

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
	FetchInoreaderSummaryUsecase        fetch_inoreader_summary_usecase.FetchInoreaderSummaryUsecase
	ImageFetchUsecase                   image_fetch_usecase.ImageFetchUsecaseInterface
	CSRFTokenUsecase                    *csrf_token_usecase.CSRFTokenUsecase
	ArticleUsecase                      fetch_article_usecase.ArticleUsecase
	ArchiveArticleUsecase               *archive_article_usecase.ArchiveArticleUsecase
	FetchArticlesCursorUsecase          *fetch_articles_usecase.FetchArticlesCursorUsecase
	FetchRecentArticlesUsecase          *fetch_recent_articles_usecase.FetchRecentArticlesUsecase
	RecapArticlesUsecase                *recap_articles_usecase.RecapArticlesUsecase
	RecapUsecase                        *recap_usecase.RecapUsecase
	MorningUsecase                      morning_letter_port.MorningUsecase
	MorningLetterUsecase                morning_letter_port.MorningLetterUsecase
	ScrapingDomainUsecase               *scraping_domain_usecase.ScrapingDomainUsecase
	BatchArticleFetcher                 *batch_article_fetcher.BatchArticleFetcher
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

	// OPML
	ExportOPMLUsecase *opml_usecase.ExportOPMLUsecase
	ImportOPMLUsecase *opml_usecase.ImportOPMLUsecase

	// Image Proxy
	ImageProxyUsecase *image_proxy_usecase.ImageProxyUsecase

	// Service-to-service Connect-RPC clients
	PreProcessorConnectClient *preprocessor_connect.ConnectPreProcessorClient

	// Internal API gateway (service-to-service)
	InternalArticleGateway *internal_article_gateway.Gateway

	// Knowledge Home
	GetKnowledgeHomeUsecase          *get_knowledge_home_usecase.GetKnowledgeHomeUsecase
	TrackHomeSeenUsecase             *track_home_seen_usecase.TrackHomeSeenUsecase
	TrackHomeActionUsecase           *track_home_action_usecase.TrackHomeActionUsecase
	AppendKnowledgeEventUsecase      *append_knowledge_event_usecase.AppendKnowledgeEventUsecase
	CreateSummaryVersionUsecase      *create_summary_version_usecase.CreateSummaryVersionUsecase
	CreateTagSetVersionUsecase       *create_tag_set_version_usecase.CreateTagSetVersionUsecase
	FeatureFlagGateway               *feature_flag_gateway.Gateway
	KnowledgeBackfillArticlesGateway *knowledge_backfill_gateway.Gateway
	KnowledgeBackfillUsecase         *knowledge_backfill_usecase.Usecase
	KnowledgeProjectionHealthUsecase *knowledge_projection_health_usecase.Usecase
	ReprojectUsecase                 *knowledge_reproject_usecase.Usecase
	SLOUsecase                       *knowledge_slo_usecase.Usecase
	AuditUsecase                     *knowledge_audit_usecase.Usecase
	MetricsUsecase                   *knowledge_metrics_usecase.Usecase

	// Phase 4: RecallRail, Lens, Stream, Supersede
	RecallRailUsecase     *recall_rail_usecase.RecallRailUsecase
	RecallSnoozeUsecase   *recall_snooze_usecase.RecallSnoozeUsecase
	RecallDismissUsecase  *recall_dismiss_usecase.RecallDismissUsecase
	CreateLensUsecase     *create_lens_usecase.CreateLensUsecase
	UpdateLensUsecase     *update_lens_usecase.UpdateLensUsecase
	ListLensesUsecase     *list_lenses_usecase.ListLensesUsecase
	SelectLensUsecase     *select_lens_usecase.SelectLensUsecase
	ArchiveLensUsecase    *archive_lens_usecase.ArchiveLensUsecase
	SummaryVersionGateway *summary_version_gateway.Gateway
	TagSetVersionGateway  *tag_set_version_gateway.Gateway

	// Knowledge Loop (new projection; see ADR-000831)
	GetKnowledgeLoopUsecase        *knowledge_loop_usecase.GetKnowledgeLoopUsecase
	TransitionKnowledgeLoopUsecase *knowledge_loop_usecase.TransitionKnowledgeLoopUsecase

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

func NewApplicationComponents(pool *pgxpool.Pool) *ApplicationComponents {
	// 1. Infrastructure (shared deps)
	infra := newInfraModule(pool)

	// 2. Subscription module (needed by feed module for auto-subscribe)
	sub := newSubscriptionModule(infra)

	// 3. Feed module (depends on subscription module for auto-subscribe wiring)
	feed := newFeedModule(infra, sub)
	feed.DeleteFeedLinkUsecase = sub.DeleteFeedLinkUsecase

	// 4. RAG module (needed by article module for ragAdapter)
	rag := newRAGModule(infra, feed)

	// 5. Article module (depends on feed + rag adapter)
	article := newArticleModule(infra, feed, rag.RagAdapter)

	// 6. Knowledge module (depends on article for InternalArticleGateway)
	knowledge := newKnowledgeModule(infra, article)

	// 7. Image module
	image := newImageModule(infra)

	// 8. Recap module
	recap := newRecapModule(infra)

	// 9. Search module (global federated search)
	search := newSearchModule(infra)

	// 10. Admin observability (gated by AdminMonitor.Enabled)
	adminMonitor := newAdminMonitorModule(infra.Config, slog.Default())

	return &ApplicationComponents{
		// Modules
		Infra:        infra,
		Feed:         feed,
		Article:      article,
		Knowledge:    knowledge,
		RAG:          rag,
		Image:        image,
		Recap:        recap,
		Search:       search,
		Subscription: sub,

		// ===== Backward-compat fields populated from modules =====

		// Ports (infra)
		ConfigPort:       infra.ConfigPort,
		RateLimiterPort:  infra.RateLimiterPort,
		ErrorHandlerPort: infra.ErrorHandler,

		// Repository
		AltDBRepository: infra.AltDBRepository,

		// Drivers
		KratosClient: infra.KratosClient,

		// Ports (RAG / event)
		RagIntegration:   rag.RagAdapter,
		RagConnectClient: rag.RagConnectClient,
		StreamChatPort:   rag.StreamChatPort,
		EventPublisher:   infra.EventPublisher,

		// Feed usecases
		FetchSingleFeedUsecase:              feed.FetchSingleFeedUsecase,
		FetchFeedsListUsecase:               feed.FetchFeedsListUsecase,
		FetchFeedsListCursorUsecase:         feed.FetchFeedsListCursorUsecase,
		FetchUnreadFeedsListCursorUsecase:   feed.FetchUnreadFeedsListCursorUsecase,
		CachedFeedListUsecase:               feed.CachedFeedListUsecase,
		FetchReadFeedsListCursorUsecase:     feed.FetchReadFeedsListCursorUsecase,
		FetchFavoriteFeedsListCursorUsecase: feed.FetchFavoriteFeedsListCursorUsecase,
		RegisterFeedsUsecase:                feed.RegisterFeedsUsecase,
		RegisterFavoriteFeedUsecase:         feed.RegisterFavoriteFeedUsecase,
		RemoveFavoriteFeedUsecase:           feed.RemoveFavoriteFeedUsecase,
		ListFeedLinksUsecase:                feed.ListFeedLinksUsecase,
		ListFeedLinksWithHealthUsecase:      feed.ListFeedLinksWithHealthUsecase,
		DeleteFeedLinkUsecase:               feed.DeleteFeedLinkUsecase,
		FeedsReadingStatusUsecase:           feed.FeedsReadingStatusUsecase,
		ArticlesReadingStatusUsecase:        feed.ArticlesReadingStatusUsecase,
		FeedsSummaryUsecase:                 feed.FeedsSummaryUsecase,
		FeedAmountUsecase:                   feed.FeedAmountUsecase,
		UnsummarizedArticlesCountUsecase:    feed.UnsummarizedArticlesCountUsecase,
		SummarizedArticlesCountUsecase:      feed.SummarizedArticlesCountUsecase,
		TotalArticlesCountUsecase:           feed.TotalArticlesCountUsecase,
		TodayUnreadArticlesCountUsecase:     feed.TodayUnreadArticlesCountUsecase,
		TrendStatsUsecase:                   feed.TrendStatsUsecase,
		FeedSearchUsecase:                   feed.FeedSearchUsecase,
		FetchFeedTagsUsecase:                feed.FetchFeedTagsUsecase,
		FetchInoreaderSummaryUsecase:        feed.FetchInoreaderSummaryUsecase,
		FetchRandomSubscriptionUsecase:      feed.FetchRandomSubscriptionUsecase,
		ScrapingDomainUsecase:               feed.ScrapingDomainUsecase,

		// Article usecases
		ArticleUsecase:             article.ArticleUsecase,
		ArchiveArticleUsecase:      article.ArchiveArticleUsecase,
		FetchArticlesCursorUsecase: article.FetchArticlesCursorUsecase,
		FetchArticleTagsUsecase:    article.FetchArticleTagsUsecase,
		FetchArticlesByTagUsecase:  article.FetchArticlesByTagUsecase,
		FetchLatestArticleUsecase:  article.FetchLatestArticleUsecase,
		FetchArticleSummaryUsecase: article.FetchArticleSummaryUsecase,
		StreamArticleTagsUsecase:   article.StreamArticleTagsUsecase,
		FetchRecentArticlesUsecase: article.FetchRecentArticlesUsecase,
		ArticleSearchUsecase:       article.ArticleSearchUsecase,
		BatchArticleFetcher:        article.BatchArticleFetcher,
		FetchTagCloudUsecase:       article.FetchTagCloudUsecase,
		InternalArticleGateway:     article.InternalArticleGateway,

		// RAG usecases
		RetrieveContextUsecase: rag.RetrieveContextUsecase,
		AnswerChatUsecase:      rag.AnswerChatUsecase,
		MorningUsecase:         rag.MorningUsecase,
		MorningLetterUsecase:   rag.MorningLetterUsecase,

		// Image usecases
		ImageFetchUsecase: image.ImageFetchUsecase,
		ImageProxyUsecase: image.ImageProxyUsecase,

		// Recap / Dashboard usecases
		RecapArticlesUsecase:    recap.RecapArticlesUsecase,
		RecapUsecase:            recap.RecapUsecase,
		GetRecapJobsUsecase:     recap.GetRecapJobsUsecase,
		DashboardMetricsUsecase: recap.DashboardMetricsUsecase,

		// Subscription / OPML / CSRF usecases
		ListSubscriptionsUsecase: sub.ListSubscriptionsUsecase,
		SubscribeUsecase:         sub.SubscribeUsecase,
		UnsubscribeUsecase:       sub.UnsubscribeUsecase,
		ExportOPMLUsecase:        sub.ExportOPMLUsecase,
		ImportOPMLUsecase:        sub.ImportOPMLUsecase,
		CSRFTokenUsecase:         sub.CSRFTokenUsecase,

		// Service-to-service Connect-RPC clients
		PreProcessorConnectClient: preprocessor_connect.NewConnectPreProcessorClient(infra.Config.PreProcessor.ConnectURL, ""),

		// Knowledge Home
		GetKnowledgeHomeUsecase:          knowledge.GetKnowledgeHomeUsecase,
		TrackHomeSeenUsecase:             knowledge.TrackHomeSeenUsecase,
		TrackHomeActionUsecase:           knowledge.TrackHomeActionUsecase,
		AppendKnowledgeEventUsecase:      knowledge.AppendKnowledgeEventUsecase,
		CreateSummaryVersionUsecase:      knowledge.CreateSummaryVersionUsecase,
		CreateTagSetVersionUsecase:       knowledge.CreateTagSetVersionUsecase,
		FeatureFlagGateway:               knowledge.FeatureFlagGateway,
		KnowledgeBackfillArticlesGateway: knowledge.KnowledgeBackfillArticlesGateway,
		KnowledgeBackfillUsecase:         knowledge.KnowledgeBackfillUsecase,
		KnowledgeProjectionHealthUsecase: knowledge.KnowledgeProjectionHealthUsecase,
		ReprojectUsecase:                 knowledge.ReprojectUsecase,
		SLOUsecase:                       knowledge.SLOUsecase,
		AuditUsecase:                     knowledge.AuditUsecase,
		MetricsUsecase:                   knowledge.MetricsUsecase,

		// RecallRail, Lens
		RecallRailUsecase:     knowledge.RecallRailUsecase,
		RecallSnoozeUsecase:   knowledge.RecallSnoozeUsecase,
		RecallDismissUsecase:  knowledge.RecallDismissUsecase,
		CreateLensUsecase:     knowledge.CreateLensUsecase,
		UpdateLensUsecase:     knowledge.UpdateLensUsecase,
		ListLensesUsecase:     knowledge.ListLensesUsecase,
		SelectLensUsecase:     knowledge.SelectLensUsecase,
		ArchiveLensUsecase:    knowledge.ArchiveLensUsecase,
		SummaryVersionGateway: knowledge.SummaryVersionGateway,
		TagSetVersionGateway:  knowledge.TagSetVersionGateway,

		// Knowledge Loop (new projection; see ADR-000831)
		GetKnowledgeLoopUsecase:        knowledge.GetKnowledgeLoopUsecase,
		TransitionKnowledgeLoopUsecase: knowledge.TransitionKnowledgeLoopUsecase,

		// Knowledge Sovereign (all knowledge data access)
		SovereignClient: knowledge.SovereignClient,

		// Observability
		KnowledgeHomeMetrics: knowledge.KnowledgeHomeMetrics,

		// Admin observability
		AdminMonitor: adminMonitor,
	}
}
