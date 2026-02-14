/**
 * Barrel re-export for backward compatibility.
 * Actual implementations are in src/lib/server/ modules.
 */

// Auth
export { getBackendToken, getCSRFToken } from "$lib/server/auth";

// REST client
export { callBackendAPIWithBody } from "$lib/server/backend-rest-client";

// Feed API
export {
	getFeedStats,
	getTodayUnreadCount,
	getFeedsWithCursor,
	getReadFeedsWithCursor,
	updateFeedReadStatus,
	getFeedLinks,
	registerRssFeed,
	deleteFeedLink,
	type DetailedFeedStats,
	type UnreadCount,
	type CursorResponse,
} from "$lib/server/feed-api";

// Tag Trail API
export {
	getRandomSubscription,
	getArticlesByTag,
	getArticleTags,
	getFeedTagsById,
	type RandomFeedResponse,
	type ArticlesByTagResponse,
	type ArticleTagsResponse,
	type FeedTagsResponse,
} from "$lib/server/tag-trail-api";
