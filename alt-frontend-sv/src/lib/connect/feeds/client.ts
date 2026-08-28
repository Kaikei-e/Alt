/**
 * FeedService client creation and shared types/helpers
 */

import type { Client, Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import { FeedService } from "$lib/gen/alt/feeds/v2/feeds_pb";

/** Type-safe FeedService client */
type FeedClient = Client<typeof FeedService>;

/**
 * Feed item from Connect-RPC (converted from proto)
 */
export interface ConnectFeedItem {
	/**
	 * List-keying identity: articles.id, or a UUID the server minted for this
	 * response. NOT a feeds.id — see `feedId` for that.
	 */
	id: string;
	title: string;
	description: string;
	link: string;
	published: string;
	createdAt: string;
	author: string;
	/** Article ID in the articles table. Undefined if article doesn't exist. */
	articleId?: string;
	/** Whether this feed has been read by the current user. */
	isRead: boolean;
	/** OGP image URL extracted from RSS feed. */
	ogImageUrl?: string;
	/** HMAC-signed proxy URL for the OGP image (pre-warmed cache). */
	ogImageProxyUrl?: string;
	/**
	 * feeds.id — the only identifier `ResolveOgImages` matches.
	 *
	 * Kept apart from `id` because the two name different tables: the resolver
	 * queries feeds, while `id` is articles.id or a per-response UUID. Absent on
	 * surfaces with no feeds.id to give (search results come from Meilisearch
	 * hits), which is why it is optional and why the resolver must be handed
	 * `feedId ?? ""` rather than `id`.
	 */
	feedId?: string;
}

/**
 * Cursor response for feed lists
 */
export interface FeedCursorResponse {
	data: ConnectFeedItem[];
	nextCursor: string | null;
	hasMore: boolean;
}

/**
 * Search response with offset-based pagination
 */
export interface FeedSearchResponse {
	data: ConnectFeedItem[];
	nextCursor: number | null;
	hasMore: boolean;
}

/**
 * Creates a FeedService client with the given transport.
 */
export function createFeedClient(transport: Transport): FeedClient {
	return createClient(FeedService, transport);
}

/**
 * Convert proto FeedItem to ConnectFeedItem.
 */
export function convertProtoFeed(proto: {
	id: string;
	title: string;
	description: string;
	link: string;
	published: string;
	createdAt: string;
	author: string;
	articleId?: string;
	isRead: boolean;
	ogImageUrl?: string;
	ogImageProxyUrl?: string;
	feedId?: string;
}): ConnectFeedItem {
	return {
		id: proto.id,
		feedId: proto.feedId || undefined,
		title: proto.title,
		description: proto.description,
		link: proto.link,
		published: proto.published,
		createdAt: proto.createdAt,
		author: proto.author,
		articleId: proto.articleId || undefined,
		isRead: proto.isRead,
		ogImageUrl: proto.ogImageUrl || undefined,
		ogImageProxyUrl: proto.ogImageProxyUrl || undefined,
	};
}

/**
 * Normalizes a URL by removing trailing slash (except for root path).
 * This ensures consistent URL comparison across frontend and backend.
 */
export function normalizeUrl(url: string): string {
	if (url.endsWith("/") && url !== "/") {
		return url.slice(0, -1);
	}
	return url;
}
