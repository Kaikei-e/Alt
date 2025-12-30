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
} from "./feeds";
