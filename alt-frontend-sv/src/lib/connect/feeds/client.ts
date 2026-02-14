/**
 * FeedService client creation and shared types/helpers
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import { FeedService } from "$lib/gen/alt/feeds/v2/feeds_pb";

/** Type-safe FeedService client */
type FeedClient = Client<typeof FeedService>;

/**
 * Feed item from Connect-RPC (converted from proto)
 */
export interface ConnectFeedItem {
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
}): ConnectFeedItem {
	return {
		id: proto.id,
		title: proto.title,
		description: proto.description,
		link: proto.link,
		published: proto.published,
		createdAt: proto.createdAt,
		author: proto.author,
		articleId: proto.articleId || undefined,
		isRead: proto.isRead,
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
