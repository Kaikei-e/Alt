import { createServerTransport } from "$lib/connect/transport-server";
import {
	getUnreadFeeds as getUnreadFeedsConnect,
	getReadFeeds as getReadFeedsConnect,
	type ConnectFeedItem,
} from "$lib/connect/feeds";
import { callBackendAPI, callBackendAPIWithBody } from "./backend-rest-client";

export interface DetailedFeedStats {
	feed_amount: { amount: number };
	total_articles: { amount: number };
	unsummarized_articles: { amount: number };
}

export interface UnreadCount {
	count: number;
}

export interface CursorResponse<T> {
	data: T[];
	next_cursor: string | null;
	has_more?: boolean;
}

/**
 * ConnectFeedItem を BackendFeedItem 互換形式に変換
 */
function connectFeedToBackendFormat(item: ConnectFeedItem): unknown {
	return {
		title: item.title,
		description: item.description,
		link: item.link,
		published: item.createdAt, // Use createdAt for RFC3339 format
		created_at: item.createdAt,
		author: item.author ? { name: item.author } : undefined,
		// Article ID in the articles table - required for mark-as-read functionality
		article_id: item.articleId,
	};
}

/**
 * 詳細統計を取得
 */
export async function getFeedStats(
	cookie: string | null,
): Promise<DetailedFeedStats> {
	return callBackendAPI<DetailedFeedStats>("/v1/feeds/stats/detailed", cookie);
}

/**
 * 今日の未読数を取得
 */
export async function getTodayUnreadCount(
	cookie: string | null,
	since: string,
): Promise<UnreadCount> {
	return callBackendAPI<UnreadCount>(
		`/v1/feeds/count/unreads?since=${encodeURIComponent(since)}`,
		cookie,
	);
}

/**
 * カーソルベースでフィードを取得
 * Connect-RPC を使用
 */
export async function getFeedsWithCursor(
	cookie: string | null,
	cursor?: string,
	limit: number = 20,
): Promise<CursorResponse<unknown>> {
	const transport = await createServerTransport(cookie);
	const response = await getUnreadFeedsConnect(transport, cursor, limit);

	return {
		data: response.data.map(connectFeedToBackendFormat),
		next_cursor: response.nextCursor,
		has_more: response.hasMore,
	};
}

/**
 * カーソルベースで既読フィードを取得
 * Connect-RPC を使用
 */
export async function getReadFeedsWithCursor(
	cookie: string | null,
	cursor?: string,
	limit: number = 32,
): Promise<CursorResponse<unknown>> {
	const transport = await createServerTransport(cookie);
	const response = await getReadFeedsConnect(transport, cursor, limit);

	return {
		data: response.data.map(connectFeedToBackendFormat),
		next_cursor: response.nextCursor,
		has_more: response.hasMore,
	};
}

/**
 * フィードを既読にする
 */
export async function updateFeedReadStatus(
	cookie: string | null,
	feedUrl: string,
): Promise<void> {
	return callBackendAPIWithBody("/v1/feeds/read", cookie, "POST", {
		feed_url: feedUrl,
	});
}

/**
 * RSSフィードリンク一覧を取得
 */
export async function getFeedLinks(
	cookie: string | null,
): Promise<import("$lib/schema/feedLink").FeedLink[]> {
	return callBackendAPI<import("$lib/schema/feedLink").FeedLink[]>(
		"/v1/rss-feed-link/list",
		cookie,
	);
}

/**
 * RSSフィードリンクを登録
 */
export async function registerRssFeed(
	cookie: string | null,
	url: string,
): Promise<void> {
	return callBackendAPIWithBody(
		"/v1/rss-feed-link/register",
		cookie,
		"POST",
		{ url },
	);
}

/**
 * RSSフィードリンクを削除
 */
export async function deleteFeedLink(
	cookie: string | null,
	id: string,
): Promise<void> {
	return callBackendAPIWithBody(
		`/v1/rss-feed-link/${encodeURIComponent(id)}`,
		cookie,
		"DELETE",
	);
}
