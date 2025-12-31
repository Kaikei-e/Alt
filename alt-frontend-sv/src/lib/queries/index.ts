/**
 * TanStack Query hooks exports for Connect-RPC clients
 */

// Query keys
export { feedKeys, articleKeys, rssKeys } from "./keys";

// Feed queries and mutations
export {
	// Stats
	createFeedStatsQuery,
	createDetailedFeedStatsQuery,
	createUnreadCountQuery,
	// Lists
	createUnreadFeedsQuery,
	createReadFeedsQuery,
	createFavoriteFeedsQuery,
	// Search
	createSearchFeedsQuery,
	// Mutations
	createMarkAsReadMutation,
	// Helpers
	flattenFeedPages,
	flattenSearchPages,
} from "./feeds";

// Article queries and mutations
export {
	createFetchArticleContentQuery,
	createArchiveArticleMutation,
	createArticlesCursorQuery,
	flattenArticlePages,
} from "./articles";

// RSS queries and mutations
export {
	createRSSLinksQuery,
	createRegisterRSSMutation,
	createDeleteRSSMutation,
	createRegisterFavoriteMutation,
} from "./rss";
