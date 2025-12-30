/**
 * FeedService client for Connect-RPC
 *
 * Provides type-safe methods to call FeedService endpoints.
 */

import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import { FeedService } from "$lib/gen/alt/feeds/v2/feeds_pb";

/**
 * Feed stats response (converted from bigint to number for convenience)
 */
export interface FeedStats {
	feedAmount: number;
	summarizedFeedAmount: number;
}

/**
 * Detailed feed stats response (converted from bigint to number for convenience)
 */
export interface DetailedFeedStats {
	feedAmount: number;
	articleAmount: number;
	unsummarizedFeedAmount: number;
}

/**
 * Unread count response (converted from bigint to number for convenience)
 */
export interface UnreadCount {
	count: number;
}

/**
 * Creates a FeedService client with the given transport.
 */
export function createFeedClient(transport: Transport) {
	return createClient(FeedService, transport);
}

/**
 * Gets basic feed statistics via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @returns Feed stats with feed count and summarized count
 */
export async function getFeedStats(transport: Transport): Promise<FeedStats> {
	const client = createFeedClient(transport);
	const response = await client.getFeedStats({});

	return {
		feedAmount: Number(response.feedAmount),
		summarizedFeedAmount: Number(response.summarizedFeedAmount),
	};
}

/**
 * Gets detailed feed statistics via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @returns Detailed feed stats with feed, article, and unsummarized counts
 */
export async function getDetailedFeedStats(
	transport: Transport,
): Promise<DetailedFeedStats> {
	const client = createFeedClient(transport);
	const response = await client.getDetailedFeedStats({});

	return {
		feedAmount: Number(response.feedAmount),
		articleAmount: Number(response.articleAmount),
		unsummarizedFeedAmount: Number(response.unsummarizedFeedAmount),
	};
}

/**
 * Gets the count of unread articles for today via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @returns The unread article count
 */
export async function getUnreadCount(
	transport: Transport,
): Promise<UnreadCount> {
	const client = createFeedClient(transport);
	const response = await client.getUnreadCount({});

	return {
		count: Number(response.count),
	};
}

// =============================================================================
// Feed List Types and Functions (Phase 2)
// =============================================================================

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
 * Get unread feeds with cursor-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 20)
 * @param view - Optional view mode ("swipe" for single-card response)
 * @returns Unread feeds with pagination info
 */
export async function getUnreadFeeds(
	transport: Transport,
	cursor?: string,
	limit: number = 20,
	view?: "swipe",
): Promise<FeedCursorResponse> {
	const client = createFeedClient(transport);
	const response = await client.getUnreadFeeds({
		cursor,
		limit,
		view,
	});

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

/**
 * Get read/viewed feeds with cursor-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 32)
 * @returns Read feeds with pagination info
 */
export async function getReadFeeds(
	transport: Transport,
	cursor?: string,
	limit: number = 32,
): Promise<FeedCursorResponse> {
	const client = createFeedClient(transport);
	const response = await client.getReadFeeds({
		cursor,
		limit,
	});

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

/**
 * Get favorite feeds with cursor-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 20)
 * @returns Favorite feeds with pagination info
 */
export async function getFavoriteFeeds(
	transport: Transport,
	cursor?: string,
	limit: number = 20,
): Promise<FeedCursorResponse> {
	const client = createFeedClient(transport);
	const response = await client.getFavoriteFeeds({
		cursor,
		limit,
	});

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

// =============================================================================
// Feed Search Types and Functions (Phase 3)
// =============================================================================

/**
 * Search feeds with offset-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param query - Search query string
 * @param cursor - Optional offset cursor for pagination
 * @param limit - Maximum number of items to return (default: 20)
 * @returns Search results with pagination info
 */
export async function searchFeeds(
	transport: Transport,
	query: string,
	cursor?: number,
	limit: number = 20,
): Promise<FeedSearchResponse> {
	const client = createFeedClient(transport);
	const response = await client.searchFeeds({
		query,
		cursor,
		limit,
	});

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Convert proto FeedItem to ConnectFeedItem.
 */
function convertProtoFeed(proto: {
	id: string;
	title: string;
	description: string;
	link: string;
	published: string;
	createdAt: string;
	author: string;
}): ConnectFeedItem {
	return {
		id: proto.id,
		title: proto.title,
		description: proto.description,
		link: proto.link,
		published: proto.published,
		createdAt: proto.createdAt,
		author: proto.author,
	};
}

// =============================================================================
// Streaming Types and Functions
// =============================================================================

/**
 * Streaming feed stats via Server Streaming RPC.
 */
export interface StreamingFeedStats {
	feedAmount: number;
	unsummarizedFeedAmount: number;
	totalArticles: number;
	isHeartbeat: boolean;
	timestamp: number;
}

/**
 * Stream feed statistics in real-time via Connect-RPC Server Streaming.
 *
 * @param transport - The Connect transport to use
 * @param onData - Callback when new stats are received
 * @param onError - Callback on error (optional)
 * @returns AbortController to cancel the stream
 */
export async function streamFeedStats(
	transport: Transport,
	onData: (stats: StreamingFeedStats) => void,
	onError?: (error: Error) => void,
): Promise<AbortController> {
	console.log("[streamFeedStats] Starting stream...");
	const client = createFeedClient(transport);
	const abortController = new AbortController();

	// Start streaming in background
	(async () => {
		try {
			console.log("[streamFeedStats] Calling client.streamFeedStats()...");
			const stream = client.streamFeedStats(
				{},
				{ signal: abortController.signal },
			);

			console.log("[streamFeedStats] Stream created, waiting for data...");
			for await (const response of stream) {
				const isHeartbeat = response.metadata?.isHeartbeat ?? false;
				console.log("[streamFeedStats] Received response:", { isHeartbeat, feedAmount: response.feedAmount });

				// Always call onData, even for heartbeats
				// Components can decide whether to ignore heartbeats
				onData({
					feedAmount: Number(response.feedAmount),
					unsummarizedFeedAmount: Number(response.unsummarizedFeedAmount),
					totalArticles: Number(response.totalArticles),
					isHeartbeat,
					timestamp: Number(response.metadata?.timestamp ?? Date.now() / 1000),
				});
			}
			console.log("[streamFeedStats] Stream ended normally");
		} catch (error) {
			console.error("[streamFeedStats] Stream error:", error);
			// Only report error if not aborted
			if (!abortController.signal.aborted && onError && error instanceof Error) {
				onError(error);
			}
		}
	})();

	return abortController;
}
