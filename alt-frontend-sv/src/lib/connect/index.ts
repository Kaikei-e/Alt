/**
 * Connect-RPC client exports for alt-frontend-sv
 *
 * NOTE: Server-side transport (createServerTransport) must be imported directly
 * from "$lib/connect/transport-server" to avoid bundling $env/dynamic/private
 * in browser code.
 */

// Re-export generated Morning Letter types for consumers
export type {
	MorningLetterBody,
	MorningLetterDocument,
	MorningLetterSection,
	MorningLetterSourceProto,
	MorningLetterSourceType,
} from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";
// ArticleService client (Phase 4)
export {
	type ArchiveArticleResult,
	type ArticleCursorResponse,
	archiveArticle,
	type ConnectArticleItem,
	createArticleClient,
	type FetchArticleContentResult,
	fetchArticleContent,
	// Tag Trail (ADR-169)
	fetchArticlesByTag,
	fetchArticlesCursor,
	fetchArticleTags,
	fetchRandomFeed,
	// Tag Verse (Tag Cloud)
	fetchTagCloud,
	type RandomFeed,
	type StreamArticleTagsResponseType,
	type StreamingStreamArticleTagsResponse,
	// Streaming Tag Trail
	streamArticleTags,
	type TagCloudItem,
	type TagTrailArticle,
	type TagTrailArticlesResponse,
	type TagTrailTag,
} from "./articles";
// AugurService client (RAG-powered Chat)
export {
	type AugurChatMessage,
	type AugurCitation,
	type AugurContextItem,
	type AugurConversationSummary,
	type AugurStoredConversation,
	type AugurStoredMessage,
	type AugurStreamOptions,
	type AugurStreamResult,
	createAugurClient,
	deleteAugurConversation,
	getAugurConversation,
	listAugurConversations,
	type RetrieveContextOptions,
	retrieveAugurContext,
	streamAugurChat,
	streamAugurChatAsync,
} from "./augur";
// Evening Pulse client
export { getEveningPulse } from "./evening_pulse";
// FeedService client
export {
	type ConnectFeedItem,
	type ConnectFeedSource,
	createFeedClient,
	type DetailedFeedStats,
	type FeedCursorResponse,
	type FeedSearchResponse,
	type FeedStats,
	getDetailedFeedStats,
	getFavoriteFeeds,
	// Phase 1: Stats
	getFeedStats,
	getReadFeeds,
	getUnreadCount,
	// Phase 2: Feed List
	getUnreadFeeds,
	listSubscriptions,
	type MarkAsReadResult,
	// Phase 7: Mark As Read
	markAsRead,
	type StreamingFeedStats,
	type StreamSummarizeChunk,
	type StreamSummarizeOptions,
	type StreamSummarizeResult,
	// Phase 3: Search
	searchFeeds,
	streamFeedStats,
	// Phase 6: Streaming Summarize
	streamSummarize,
	streamSummarizeWithAbort,
	type UnreadCount,
} from "./feeds";
// GlobalSearchService client
export {
	type ArticleSectionData,
	createGlobalSearchClient,
	type GlobalArticleHitData,
	type GlobalRecapHitData,
	type GlobalSearchOptions,
	type GlobalSearchResult,
	type GlobalTagHitData,
	type RecapSectionData,
	searchEverything,
	type TagSectionData,
} from "./global_search";
// KnowledgeHomeService client
export {
	createKnowledgeHomeClient,
	createLens,
	deleteLens,
	type FeatureFlagData,
	getKnowledgeHome,
	type KnowledgeHomeItemData,
	type KnowledgeHomeResult,
	type LensData,
	type LensVersionData,
	type ListLensesResult,
	listLenses,
	type RecallCandidateData,
	type RecallReasonData,
	type ServiceQuality,
	type StreamHomeUpdate,
	type SupersedeInfoData,
	selectLens,
	type TodayDigestData,
	trackHomeAction,
	trackHomeItemsSeen,
	type WhyReasonData,
} from "./knowledge_home";
// KnowledgeHomeAdminService client
export {
	type AlertSummaryData,
	type BackfillJobData,
	compareReproject,
	type FeatureFlagsConfigData,
	getFeatureFlags,
	getProjectionHealth,
	getSLOStatus,
	type KnowledgeHomeAdminData,
	listReprojectRuns,
	type ProjectionHealthData,
	pauseBackfill,
	type ReprojectDiffSummaryData,
	type ReprojectRunData,
	resumeBackfill,
	rollbackReproject,
	type SLIStatusData,
	type SLOStatusData,
	startReproject,
	swapReproject,
	triggerBackfill,
} from "./knowledge_home_admin";
// MorningLetterService client (Time-bounded RAG Chat)
export {
	createMorningLetterClient,
	// MorningLetterReadService (Document-oriented read APIs)
	getLatestLetter,
	getLetterByDate,
	getLetterEnrichment,
	getLetterSources,
	type MorningLetterChatMessage,
	type MorningLetterCitation,
	type MorningLetterMeta,
	type MorningLetterStreamOptions,
	type MorningLetterStreamResult,
	type MorningLetterTimeWindow,
	regenerateLatestLetter,
	streamMorningLetterChat,
	streamMorningLetterChatAsync,
} from "./morning_letter";
// RecapService client
export {
	createRecapClient,
	getSevenDayRecap,
	getThreeDayRecap,
	type RecapGenreWithReferences,
	type RecapReference,
	type RecapSearchResultItem,
	type RecapSummaryWithReferences,
	searchRecaps,
	searchRecapsByTag,
} from "./recap";
// RSSService client (Phase 5)
export {
	createRSSClient,
	type DeleteRSSFeedLinkResult,
	deleteRSSFeedLink,
	type ListRSSFeedLinksResult,
	listRSSFeedLinks,
	type RegisterFavoriteFeedResult,
	type RegisterRSSFeedResult,
	type RSSFeedLink,
	registerFavoriteFeed,
	registerRSSFeed,
} from "./rss";
// Streaming Adapter (Connect-RPC to Renderer bridge)
export {
	type StreamSummarizeAdapterOptions,
	type StreamSummarizeAdapterResult,
	streamSummarizeWithAbortAdapter,
	streamSummarizeWithRenderer,
} from "./streamingAdapter";
// Client-side transport (safe for browser)
export { createClientTransport } from "./transport-client";
