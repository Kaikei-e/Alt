/**
 * Connect-RPC client exports for alt-frontend-sv
 */

// Transport utilities
export {
	createServerTransport,
	createClientTransport,
} from "./transport";

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
