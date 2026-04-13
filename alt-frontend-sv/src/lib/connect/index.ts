/**
 * Connect-RPC client exports for alt-frontend-sv
 *
 * NOTE: Server-side transport (createServerTransport) must be imported directly
 * from "$lib/connect/transport-server" to avoid bundling $env/dynamic/private
 * in browser code.
 */

// Client-side transport (safe for browser)
export { createClientTransport } from "./transport-client";

// FeedService client
export {
	createFeedClient,
	// Phase 1: Stats
	getFeedStats,
	getDetailedFeedStats,
	getUnreadCount,
	streamFeedStats,
	type FeedStats,
	type DetailedFeedStats,
	type UnreadCount,
	type StreamingFeedStats,
	// Phase 2: Feed List
	getUnreadFeeds,
	getReadFeeds,
	getFavoriteFeeds,
	type ConnectFeedItem,
	type FeedCursorResponse,
	// Phase 3: Search
	searchFeeds,
	type FeedSearchResponse,
	// Phase 6: Streaming Summarize
	streamSummarize,
	streamSummarizeWithAbort,
	type StreamSummarizeOptions,
	type StreamSummarizeChunk,
	type StreamSummarizeResult,
	// Phase 7: Mark As Read
	markAsRead,
	listSubscriptions,
	type MarkAsReadResult,
	type ConnectFeedSource,
} from "./feeds";

// ArticleService client (Phase 4)
export {
	createArticleClient,
	fetchArticleContent,
	archiveArticle,
	fetchArticlesCursor,
	type FetchArticleContentResult,
	type ArchiveArticleResult,
	type ConnectArticleItem,
	type ArticleCursorResponse,
	// Tag Trail (ADR-169)
	fetchArticlesByTag,
	fetchArticleTags,
	fetchRandomFeed,
	// Streaming Tag Trail
	streamArticleTags,
	type StreamArticleTagsResponseType,
	type StreamingStreamArticleTagsResponse,
	type TagTrailArticle,
	type TagTrailTag,
	type RandomFeed,
	type TagTrailArticlesResponse,
	// Tag Verse (Tag Cloud)
	fetchTagCloud,
	type TagCloudItem,
} from "./articles";

// RSSService client (Phase 5)
export {
	createRSSClient,
	registerRSSFeed,
	listRSSFeedLinks,
	deleteRSSFeedLink,
	registerFavoriteFeed,
	type RSSFeedLink,
	type RegisterRSSFeedResult,
	type ListRSSFeedLinksResult,
	type DeleteRSSFeedLinkResult,
	type RegisterFavoriteFeedResult,
} from "./rss";

// Streaming Adapter (Connect-RPC to Renderer bridge)
export {
	streamSummarizeWithRenderer,
	streamSummarizeWithAbortAdapter,
	type StreamSummarizeAdapterOptions,
	type StreamSummarizeAdapterResult,
} from "./streamingAdapter";

// AugurService client (RAG-powered Chat)
export {
	createAugurClient,
	streamAugurChat,
	streamAugurChatAsync,
	retrieveAugurContext,
	listAugurConversations,
	getAugurConversation,
	deleteAugurConversation,
	type AugurCitation,
	type AugurChatMessage,
	type AugurStreamOptions,
	type AugurStreamResult,
	type AugurContextItem,
	type RetrieveContextOptions,
	type AugurConversationSummary,
	type AugurStoredMessage,
	type AugurStoredConversation,
} from "./augur";

// MorningLetterService client (Time-bounded RAG Chat)
export {
	createMorningLetterClient,
	streamMorningLetterChat,
	streamMorningLetterChatAsync,
	type MorningLetterCitation,
	type MorningLetterTimeWindow,
	type MorningLetterChatMessage,
	type MorningLetterStreamOptions,
	type MorningLetterMeta,
	type MorningLetterStreamResult,
	// MorningLetterReadService (Document-oriented read APIs)
	getLatestLetter,
	getLetterByDate,
	getLetterSources,
	getLetterEnrichment,
	regenerateLatestLetter,
} from "./morning_letter";

// Re-export generated Morning Letter types for consumers
export type {
	MorningLetterDocument,
	MorningLetterBody,
	MorningLetterSection,
	MorningLetterSourceProto,
	MorningLetterSourceType,
} from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";

// RecapService client
export {
	createRecapClient,
	getSevenDayRecap,
	getThreeDayRecap,
	searchRecaps,
	searchRecapsByTag,
	type RecapReference,
	type RecapGenreWithReferences,
	type RecapSummaryWithReferences,
	type RecapSearchResultItem,
} from "./recap";

// Evening Pulse client
export { getEveningPulse } from "./evening_pulse";

// TTSService client
export {
	createTtsClient,
	synthesizeSpeech,
	synthesizeSpeechStream,
	listVoices,
	type SynthesizeResult,
	type SynthesizeOptions,
	type TtsVoice,
} from "./tts";

// KnowledgeHomeService client
export {
	createKnowledgeHomeClient,
	getKnowledgeHome,
	trackHomeItemsSeen,
	trackHomeAction,
	getRecallRailCandidates,
	snoozeRecallItem,
	dismissRecallItem,
	listLenses,
	createLens,
	deleteLens,
	selectLens,
	type WhyReasonData,
	type TodayDigestData,
	type KnowledgeHomeItemData,
	type KnowledgeHomeResult,
	type ServiceQuality,
	type FeatureFlagData,
	type SupersedeInfoData,
	type RecallCandidateData,
	type RecallReasonData,
	type LensData,
	type LensVersionData,
	type ListLensesResult,
	type StreamHomeUpdate,
} from "./knowledge_home";

// GlobalSearchService client
export {
	createGlobalSearchClient,
	searchEverything,
	type GlobalArticleHitData,
	type GlobalRecapHitData,
	type GlobalTagHitData,
	type ArticleSectionData,
	type RecapSectionData,
	type TagSectionData,
	type GlobalSearchResult,
	type GlobalSearchOptions,
} from "./global_search";

// KnowledgeHomeAdminService client
export {
	getProjectionHealth,
	getFeatureFlags,
	triggerBackfill,
	pauseBackfill,
	resumeBackfill,
	getSLOStatus,
	listReprojectRuns,
	startReproject,
	compareReproject,
	swapReproject,
	rollbackReproject,
	type ProjectionHealthData,
	type FeatureFlagsConfigData,
	type BackfillJobData,
	type KnowledgeHomeAdminData,
	type SLOStatusData,
	type SLIStatusData,
	type AlertSummaryData,
	type ReprojectRunData,
	type ReprojectDiffSummaryData,
} from "./knowledge_home_admin";
