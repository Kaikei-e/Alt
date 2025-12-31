import type { FeedLink } from "$lib/schema/feedLink";
import { createClientTransport } from "$lib/connect/transport.client";

/**
 * RSSフィードリンク一覧を取得（クライアントサイド）
 * Connect-RPC を使用
 */
export async function listFeedLinksClient(): Promise<FeedLink[]> {
	const transport = createClientTransport();
	const { listRSSFeedLinks } = await import("$lib/connect/rss");
	const response = await listRSSFeedLinks(transport);

	return response.links.map((link) => ({
		id: link.id,
		url: link.url,
	}));
}

/**
 * RSSフィードリンクを登録（クライアントサイド）
 * Connect-RPC を使用
 */
export async function registerRssFeedClient(url: string): Promise<void> {
	const transport = createClientTransport();
	const { registerRSSFeed } = await import("$lib/connect/rss");
	await registerRSSFeed(transport, url);
}

/**
 * RSSフィードリンクを削除（クライアントサイド）
 * Connect-RPC を使用
 */
export async function deleteFeedLinkClient(id: string): Promise<void> {
	const transport = createClientTransport();
	const { deleteRSSFeedLink } = await import("$lib/connect/rss");
	await deleteRSSFeedLink(transport, id);
}
