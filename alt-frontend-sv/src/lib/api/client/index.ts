/**
 * クライアントサイドAPI呼び出しのエントリーポイント
 * 各モジュールから必要な関数を再エクスポート
 */

// 記事関連のAPI
export {
	type ArticleSummaryItem,
	archiveContentClient,
	batchPrefetchImagesClient,
	type FeedContentOnTheFlyResponse,
	type FetchArticleSummaryResponse,
	getArticleSummaryClient,
	getFeedContentOnTheFlyClient,
	type MessageResponse,
	registerFavoriteFeedClient,
	type SummarizeArticleResponse,
	summarizeArticleClient,
} from "./articles";
// 共通のAPI呼び出しロジック
export { callClientAPI } from "./core";
// フィードリンク管理関連のAPI
export {
	deleteFeedLinkClient,
	listFeedLinksClient,
	registerRssFeedClient,
} from "./feedLinks";
// フィード関連のAPI
export {
	getFavoriteFeedsWithCursorClient,
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	searchFeedsClient,
	updateFeedReadStatusClient,
	listSubscriptionsClient,
	subscribeClient,
	unsubscribeClient,
} from "./feeds";
// NOTE: Recap API migrated to Connect-RPC (see $lib/connect/recap.ts)
// Tag Trail関連のAPI
export {
	getArticlesByTagClient,
	getArticleTagsClient,
	getFeedTagsByIdClient,
	getRandomSubscriptionClient,
} from "./tagTrail";
