package di

import (
	"alt/config"
	"alt/orchestrator/driver/preprocessor_connect"
	"log/slog"
)

// NewBackendComponents is cmd/backend's composition root: the browser-facing
// REST listener, the user Connect-RPC listener, and the loopback operator
// listener.
//
// Compared with the pre-split container it deliberately does not build:
//
//   - KratosClient — only DataHubService.GetSystemUser reads it (cmd/datahub)
//   - EventPublisher — only DataHubService reads it (cmd/datahub)
//   - RecapArticlesUsecase / FetchRecentArticlesUsecase /
//     CreateTagSetVersionUsecase — data-plane reads served by cmd/datahub
//   - ConfigPort / RateLimiterPort / ErrorHandlerPort /
//     AppendKnowledgeEventUsecase — no consumer anywhere (plan R11)
//   - AltDBRepository, and any database pool at all — ADR-000954 Wave 3
//     batch 6 moved the last two reads (the Tag Trail's paging and the recall
//     rail's article fallback) onto alt-data-hub, so this root takes no pool
//     and there is nothing here to give one to
//
// Those are absent fields, not nil ones. A handler that reaches for them fails
// to compile, which is the strongest available form of "no silent fallback for
// unwired dependencies" (CLAUDE.md rule 8). For the database the guarantee is
// stronger still and is checked rather than asserted: cmd/backend's transitive
// dependency graph contains neither alt/shared/driver/alt_db nor pgx — see
// di/import_boundary_test.go.
//
// InternalArticleGateway is gone with the pool. It was the one internal-looking
// component the backend kept, for RecallRailUsecase's article fallback, and
// that read is now a procedure (catalog §2.C).
func NewBackendComponents(cfg *config.Config) *ApplicationComponents {
	// 1. Infrastructure (shared deps)
	infra := newInfraModule(cfg)

	// 2. Subscription module (needed by feed module for auto-subscribe)
	sub := newSubscriptionModule(infra)

	// 3. Feed module (depends on subscription module for auto-subscribe and
	//    for DeleteFeedLinkUsecase)
	feed := newFeedModule(infra, sub)

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

		// Ports (RAG)
		RagIntegration:   rag.RagAdapter,
		RagConnectClient: rag.RagConnectClient,
		StreamChatPort:   rag.StreamChatPort,

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
		FetchFeedTagsByIDUsecase:            feed.FetchFeedTagsByIDUsecase,
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
		ArticleSearchUsecase:       article.ArticleSearchUsecase,
		BatchArticleFetcher:        article.BatchArticleFetcher,
		FetchArticleGateway:        article.FetchArticleGateway,
		FetchTagCloudUsecase:       article.FetchTagCloudUsecase,
		GetArticleSourceURLUsecase: article.GetArticleSourceURLUsecase,

		SummarizeArticleUsecase:      article.SummarizeArticleUsecase,
		FetchArticleSummariesUsecase: article.FetchArticleSummariesUsecase,
		PreProcessorSummarizeGateway: article.PreProcessorSummarizeGateway,

		// RAG usecases
		RetrieveContextUsecase: rag.RetrieveContextUsecase,
		AnswerChatUsecase:      rag.AnswerChatUsecase,
		MorningUsecase:         rag.MorningUsecase,
		MorningLetterUsecase:   rag.MorningLetterUsecase,

		// Image usecases
		ImageFetchUsecase: image.ImageFetchUsecase,
		ImageProxyUsecase: image.ImageProxyUsecase,

		// Recap / Dashboard usecases
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
		GetKnowledgeTrailUsecase:         knowledge.GetKnowledgeTrailUsecase,
		ResolveTrailBranchUsecase:        knowledge.ResolveTrailBranchUsecase,
		EmitTrailOutcomeUsecase:          knowledge.EmitTrailOutcomeUsecase,
		SearchTrailUsecase:               knowledge.SearchTrailUsecase,
		GetItemBranchesUsecase:           knowledge.GetItemBranchesUsecase,
		TrackHomeSeenUsecase:             knowledge.TrackHomeSeenUsecase,
		TrackHomeActionUsecase:           knowledge.TrackHomeActionUsecase,
		CreateSummaryVersionUsecase:      knowledge.CreateSummaryVersionUsecase,
		FeatureFlagGateway:               knowledge.FeatureFlagGateway,
		KnowledgeBackfillUsecase:         knowledge.KnowledgeBackfillUsecase,
		KnowledgeURLBackfillUsecase:      knowledge.KnowledgeURLBackfillUsecase,
		KnowledgeProjectionHealthUsecase: knowledge.KnowledgeProjectionHealthUsecase,
		ReprojectUsecase:                 knowledge.ReprojectUsecase,
		SLOUsecase:                       knowledge.SLOUsecase,
		AuditUsecase:                     knowledge.AuditUsecase,
		MetricsUsecase:                   knowledge.MetricsUsecase,

		// RecallRail, Lens
		RecallRailUsecase:    knowledge.RecallRailUsecase,
		RecallSnoozeUsecase:  knowledge.RecallSnoozeUsecase,
		RecallDismissUsecase: knowledge.RecallDismissUsecase,
		CreateLensUsecase:    knowledge.CreateLensUsecase,
		UpdateLensUsecase:    knowledge.UpdateLensUsecase,
		ListLensesUsecase:    knowledge.ListLensesUsecase,
		SelectLensUsecase:    knowledge.SelectLensUsecase,
		ArchiveLensUsecase:   knowledge.ArchiveLensUsecase,

		// Knowledge Sovereign (all knowledge data access)
		SovereignClient: knowledge.SovereignClient,

		// Observability
		KnowledgeHomeMetrics: knowledge.KnowledgeHomeMetrics,

		// Admin observability
		AdminMonitor: adminMonitor,
	}
}
