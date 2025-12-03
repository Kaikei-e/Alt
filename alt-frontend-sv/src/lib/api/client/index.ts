/**
 * クライアントサイドAPI呼び出しのエントリーポイント
 * 各モジュールから必要な関数を再エクスポート
 */

// 共通のAPI呼び出しロジック
export { callClientAPI } from "./core";

// フィード関連のAPI
export {
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	updateFeedReadStatusClient,
} from "./feeds";

