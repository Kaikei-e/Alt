/**
 * Barrel re-export for backward compatibility.
 * Actual implementations are in src/lib/server/ modules.
 */

// Auth
export {
	getBackendToken,
	getCSRFToken,
	issueCsrfCookie,
	verifyCsrfToken,
} from "$lib/server/auth";

// REST client
export { callBackendAPIWithBody } from "$lib/server/backend-rest-client";

// Feed API
export {
	type CursorResponse,
	type DetailedFeedStats,
	deleteFeedLink,
	getFeedLinks,
	getFeedStats,
	getFeedsWithCursor,
	getReadFeedsWithCursor,
	getTodayUnreadCount,
	registerRssFeed,
	type UnreadCount,
	updateFeedReadStatus,
} from "$lib/server/feed-api";

// Tag Trail API
export {
	type ArticlesByTagResponse,
	type ArticleTagsResponse,
	type FeedTagsResponse,
	getArticlesByTag,
	getArticleTags,
	getFeedTagsById,
	getRandomSubscription,
	type RandomFeedResponse,
} from "$lib/server/tag-trail-api";
