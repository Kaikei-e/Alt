import type { CursorResponse } from "$lib/api";
import type { RenderFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import type { FeedSearchResult, SearchFeedItem } from "$lib/schema/search";
import type {
	DetailedFeedStatsSummary,
	FeedStatsSummary,
	UnreadCountResponse,
} from "$lib/schema/stats";
import { createClientTransport } from "$lib/connect/transport.client";
import {
	getUnreadFeeds,
	getAllFeeds,
	getReadFeeds,
	getFavoriteFeeds,
	searchFeeds as searchFeedsConnect,
	listSubscriptions,
	type ConnectFeedItem,
	type ConnectFeedSource,
} from "$lib/connect/feeds";
import {
	formatPublishedDate,
	generateExcerptFromDescription,
	normalizeUrl,
} from "$lib/utils/feed";

/**
 * ConnectFeedItem を RenderFeed に変換
 * バックエンドで既に sanitize/format 済みのデータを RenderFeed 形式に変換
 */
function connectFeedToRenderFeed(item: ConnectFeedItem): RenderFeed {
	return {
		id: item.id,
		title: item.title,
		description: item.description,
		link: item.link,
		published: item.published, // Already formatted as "2h ago" etc.
		created_at: item.createdAt,
		author: item.author || undefined,
		// Article ID in the articles table - used to determine if mark-as-read is available
		articleId: item.articleId,
		isRead: item.isRead,
		// Generate display values from the already-sanitized data
		publishedAtFormatted: formatPublishedDate(item.createdAt || item.published),
		mergedTagsLabel: "", // Tags not available in Connect-RPC response
		normalizedUrl: normalizeUrl(item.link),
		excerpt: generateExcerptFromDescription(item.description),
		ogImageProxyUrl: item.ogImageProxyUrl,
	};
}

/**
 * カーソルベースでフィードを取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function getFeedsWithCursorClient(
	cursor?: string,
	limit: number = 20,
	excludeFeedLinkId?: string,
): Promise<CursorResponse<RenderFeed>> {
	const transport = createClientTransport();
	const response = await getUnreadFeeds(
		transport,
		cursor,
		limit,
		undefined,
		excludeFeedLinkId,
	);

	return {
		data: response.data.map(connectFeedToRenderFeed),
		next_cursor: response.nextCursor,
		has_more: response.hasMore,
	};
}

/**
 * 全フィード（既読＋未読）をカーソルベースで取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function getAllFeedsWithCursorClient(
	cursor?: string,
	limit: number = 20,
	excludeFeedLinkId?: string,
): Promise<CursorResponse<RenderFeed>> {
	const transport = createClientTransport();
	const response = await getAllFeeds(
		transport,
		cursor,
		limit,
		excludeFeedLinkId,
	);

	return {
		data: response.data.map(connectFeedToRenderFeed),
		next_cursor: response.nextCursor,
		has_more: response.hasMore,
	};
}

/**
 * お気に入りフィードをカーソルベースで取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function getFavoriteFeedsWithCursorClient(
	cursor?: string,
	limit: number = 20,
): Promise<CursorResponse<RenderFeed>> {
	const transport = createClientTransport();
	const response = await getFavoriteFeeds(transport, cursor, limit);

	return {
		data: response.data.map(connectFeedToRenderFeed),
		next_cursor: response.nextCursor,
		has_more: response.hasMore,
	};
}

/**
 * フィードを既読にする（クライアントサイド）
 * Connect-RPC を使用
 */
export async function updateFeedReadStatusClient(
	feedUrl: string,
): Promise<void> {
	const transport = createClientTransport();
	const { markAsRead } = await import("$lib/connect/feeds");
	await markAsRead(transport, feedUrl);
}

/**
 * カーソルベースで既読フィードを取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function getReadFeedsWithCursorClient(
	cursor?: string,
	limit: number = 32,
): Promise<CursorResponse<RenderFeed>> {
	const transport = createClientTransport();
	const response = await getReadFeeds(transport, cursor, limit);

	return {
		data: response.data.map(connectFeedToRenderFeed),
		next_cursor: response.nextCursor,
		has_more: response.hasMore,
	};
}

/**
 * フィードを検索（クライアントサイド）
 * Connect-RPC を使用（offsetベースのページネーション）
 */
export async function searchFeedsClient(
	query: string,
	cursor?: number, // Offset for pagination (integer)
	limit: number = 20,
): Promise<FeedSearchResult> {
	try {
		const transport = createClientTransport();
		const response = await searchFeedsConnect(transport, query, cursor, limit);

		// Convert ConnectFeedItem[] to SearchFeedItem[]
		const results: SearchFeedItem[] = response.data.map((item) => ({
			title: item.title,
			description: item.description,
			link: item.link,
			published: item.published,
			author: item.author ? { name: item.author } : undefined,
			article_id: item.articleId,
		}));

		return {
			results,
			error: null,
			next_cursor: response.nextCursor,
			has_more: response.hasMore,
		};
	} catch (error) {
		const errorMessage =
			error instanceof Error ? error.message : "Search failed";
		return {
			results: [],
			error: errorMessage,
			next_cursor: null,
			has_more: false,
		};
	}
}

/**
 * デスクトップ検索用: RenderFeed[] を直接返す
 * connectFeedToRenderFeed を使用し、id/tags/createdAt 等を保持
 */
export async function searchFeedsDesktopClient(
	query: string,
	cursor?: number,
	limit: number = 20,
): Promise<{
	data: RenderFeed[];
	error: string | null;
	next_cursor: number | null;
	has_more: boolean;
}> {
	try {
		const transport = createClientTransport();
		const response = await searchFeedsConnect(transport, query, cursor, limit);
		return {
			data: response.data.map(connectFeedToRenderFeed),
			error: null,
			next_cursor: response.nextCursor ?? null,
			has_more: response.hasMore,
		};
	} catch (error) {
		return {
			data: [],
			error: error instanceof Error ? error.message : "Search failed",
			next_cursor: null,
			has_more: false,
		};
	}
}

/**
 * フィードの統計情報を取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function getFeedStatsClient(): Promise<FeedStatsSummary> {
	const transport = createClientTransport();
	const { getFeedStats } = await import("$lib/connect/feeds");
	const response = await getFeedStats(transport);

	return {
		feed_amount: { amount: response.feedAmount },
		summarized_feed: { amount: response.summarizedFeedAmount },
	};
}

/**
 * フィードの詳細統計情報を取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function getDetailedFeedStatsClient(): Promise<DetailedFeedStatsSummary> {
	const transport = createClientTransport();
	const { getDetailedFeedStats } = await import("$lib/connect/feeds");
	const response = await getDetailedFeedStats(transport);

	return {
		feed_amount: { amount: response.feedAmount },
		total_articles: { amount: response.articleAmount },
		unsummarized_articles: { amount: response.unsummarizedFeedAmount },
	};
}

/**
 * 未読記事数を取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function getUnreadCountClient(): Promise<UnreadCountResponse> {
	const transport = createClientTransport();
	const { getUnreadCount } = await import("$lib/connect/feeds");
	const response = await getUnreadCount(transport);

	return {
		count: response.count,
	};
}

// =============================================================================
// Subscription Management (Client-side)
// =============================================================================

/**
 * 購読ソース一覧を取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function listSubscriptionsClient(): Promise<ConnectFeedSource[]> {
	const transport = createClientTransport();
	return listSubscriptions(transport);
}

/**
 * フィードソースを購読（クライアントサイド）
 * Connect-RPC を使用
 */
export async function subscribeClient(feedLinkId: string): Promise<string> {
	const transport = createClientTransport();
	const { subscribe } = await import("$lib/connect/feeds");
	return subscribe(transport, feedLinkId);
}

/**
 * フィードソースの購読を解除（クライアントサイド）
 * Connect-RPC を使用
 */
export async function unsubscribeClient(feedLinkId: string): Promise<string> {
	const transport = createClientTransport();
	const { unsubscribe } = await import("$lib/connect/feeds");
	return unsubscribe(transport, feedLinkId);
}
