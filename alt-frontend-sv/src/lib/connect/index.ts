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
	type MarkAsReadResult,
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
	type AugurCitation,
	type AugurChatMessage,
	type AugurStreamOptions,
	type AugurStreamResult,
	type AugurContextItem,
	type RetrieveContextOptions,
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
} from "./morning_letter";

// RecapService client
export {
	createRecapClient,
	getSevenDayRecap,
	type RecapReference,
	type RecapGenreWithReferences,
	type RecapSummaryWithReferences,
} from "./recap";

// Evening Pulse client
export { getEveningPulse } from "./evening_pulse";
