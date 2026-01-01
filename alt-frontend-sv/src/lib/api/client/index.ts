/**
 * クライアントサイドAPI呼び出しのエントリーポイント
 * 各モジュールから必要な関数を再エクスポート
 */

// 記事関連のAPI
export {
	type ArticleSummaryItem,
	archiveContentClient,
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
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	searchFeedsClient,
	updateFeedReadStatusClient,
} from "./feeds";
// リキャップ関連のAPI
export { get7DaysRecapClient } from "./recap";
