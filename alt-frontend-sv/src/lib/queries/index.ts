/**
 * TanStack Query hooks exports for Connect-RPC clients
 */

// Query keys
export {
	feedKeys,
	articleKeys,
	rssKeys,
	recapKeys,
	pulseKeys,
	tagTrailKeys,
} from "./keys";

// Feed queries and mutations
export {
	createFeedStatsQuery,
	createDetailedFeedStatsQuery,
	createUnreadCountQuery,
	createUnreadFeedsQuery,
	createReadFeedsQuery,
	createFavoriteFeedsQuery,
	createSearchFeedsQuery,
	createMarkAsReadMutation,
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

// Recap queries
export {
	createThreeDayRecapQuery,
	createSevenDayRecapQuery,
} from "./recap";

// Evening Pulse queries
export { createEveningPulseQuery } from "./pulse";

// Tag Trail queries
export {
	createArticlesByTagQuery,
	createArticleTagsQuery,
	createRandomFeedQuery,
} from "./tag-trail";
