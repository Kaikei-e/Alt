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
	getFeedStats,
	getDetailedFeedStats,
	getUnreadCount,
	type FeedStats,
	type DetailedFeedStats,
	type UnreadCount,
} from "./feeds";
