/**
 * TanStack Query hooks exports for Connect-RPC clients
 */

// Article queries and mutations
export {
	createArchiveArticleMutation,
	createArticlesCursorQuery,
	createFetchArticleContentQuery,
	flattenArticlePages,
} from "./articles";

// Feed queries and mutations
export {
	createDetailedFeedStatsQuery,
	createFavoriteFeedsQuery,
	createFeedStatsQuery,
	createMarkAsReadMutation,
	createReadFeedsQuery,
	createSearchFeedsQuery,
	createUnreadCountQuery,
	createUnreadFeedsQuery,
	flattenFeedPages,
	flattenSearchPages,
} from "./feeds";
// Query keys
export {
	articleKeys,
	feedKeys,
	pulseKeys,
	recapKeys,
	rssKeys,
	tagTrailKeys,
} from "./keys";
// Evening Pulse queries
export { createEveningPulseQuery } from "./pulse";

// Recap queries
export {
	createSevenDayRecapQuery,
	createThreeDayRecapQuery,
} from "./recap";
// RSS queries and mutations
export {
	createDeleteRSSMutation,
	createRegisterFavoriteMutation,
	createRegisterRSSMutation,
	createRSSLinksQuery,
} from "./rss";

// Tag Trail queries
export {
	createArticlesByTagQuery,
	createArticleTagsQuery,
	createRandomFeedQuery,
} from "./tag-trail";
