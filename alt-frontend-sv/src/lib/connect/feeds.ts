/**
 * FeedService client barrel re-export.
 * Actual implementations are in src/lib/connect/feeds/ sub-modules.
 */

// Actions (Mark as Read, Subscriptions)
export {
	type ConnectFeedSource,
	listSubscriptions,
	type MarkAsReadResult,
	markAsRead,
	subscribe,
	unsubscribe,
} from "./feeds/actions";
// Client & shared types
export {
	type ConnectFeedItem,
	convertProtoFeed,
	createFeedClient,
	type FeedCursorResponse,
	type FeedSearchResponse,
	normalizeUrl,
} from "./feeds/client";

// Listing & Search
export {
	getAllFeeds,
	getFavoriteFeeds,
	getReadFeeds,
	getUnreadFeeds,
	searchFeeds,
} from "./feeds/listing";
// Stats
export {
	type DetailedFeedStats,
	type FeedStats,
	getDetailedFeedStats,
	getFeedStats,
	getUnreadCount,
	type UnreadCount,
} from "./feeds/stats";
// Streaming
export {
	type StreamingFeedStats,
	type StreamSummarizeChunk,
	type StreamSummarizeOptions,
	type StreamSummarizeResult,
	streamFeedStats,
	streamSummarize,
	streamSummarizeWithAbort,
} from "./feeds/streaming";
