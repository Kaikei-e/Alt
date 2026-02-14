/**
 * FeedService client barrel re-export.
 * Actual implementations are in src/lib/connect/feeds/ sub-modules.
 */

// Client & shared types
export {
	createFeedClient,
	convertProtoFeed,
	normalizeUrl,
	type ConnectFeedItem,
	type FeedCursorResponse,
	type FeedSearchResponse,
} from "./feeds/client";

// Stats
export {
	getFeedStats,
	getDetailedFeedStats,
	getUnreadCount,
	type FeedStats,
	type DetailedFeedStats,
	type UnreadCount,
} from "./feeds/stats";

// Listing & Search
export {
	getUnreadFeeds,
	getAllFeeds,
	getReadFeeds,
	getFavoriteFeeds,
	searchFeeds,
} from "./feeds/listing";

// Streaming
export {
	streamFeedStats,
	streamSummarize,
	streamSummarizeWithAbort,
	type StreamingFeedStats,
	type StreamSummarizeOptions,
	type StreamSummarizeChunk,
	type StreamSummarizeResult,
} from "./feeds/streaming";

// Actions (Mark as Read, Subscriptions)
export {
	markAsRead,
	listSubscriptions,
	subscribe,
	unsubscribe,
	type MarkAsReadResult,
	type ConnectFeedSource,
} from "./feeds/actions";
