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
	/** Article ID in the articles table. Undefined if article doesn't exist. */
	articleId?: string;
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
	articleId?: string;
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
			// Check abort BEFORE logging to suppress navigation-related errors
			if (abortController.signal.aborted) {
				console.log("[streamFeedStats] Stream closed due to navigation");
				return;
			}
			console.error("[streamFeedStats] Stream error:", error);
			if (onError && error instanceof Error) {
				onError(error);
			}
		}
	})();

	return abortController;
}

// =============================================================================
// Streaming Summarize Types and Functions (Phase 6)
// =============================================================================

/**
 * Request options for streaming summarization.
 */
export interface StreamSummarizeOptions {
	/** Feed/article URL (required if articleId not provided) */
	feedUrl?: string;
	/** Existing article ID (required if feedUrl not provided) */
	articleId?: string;
	/** Pre-fetched content (optional, skips fetch if provided) */
	content?: string;
	/** Article title (optional) */
	title?: string;
}

/**
 * Streaming summarize chunk response.
 */
export interface StreamSummarizeChunk {
	/** Text chunk from summarization */
	chunk: string;
	/** Whether this is the final chunk */
	isFinal: boolean;
	/** Article ID (populated after first chunk or from cache) */
	articleId: string;
	/** Whether this response is from cache */
	isCached: boolean;
	/** Full summary (only populated if isCached=true or isFinal=true) */
	fullSummary: string | null;
}

/**
 * Result returned when streaming completes successfully.
 */
export interface StreamSummarizeResult {
	/** The article ID */
	articleId: string;
	/** The full summary text */
	summary: string;
	/** Whether the result was from cache */
	wasCached: boolean;
}

/**
 * Stream article summarization in real-time via Connect-RPC Server Streaming.
 *
 * @param transport - The Connect transport to use
 * @param options - Request options (feedUrl or articleId required)
 * @param onChunk - Callback when a chunk is received (optional)
 * @param onError - Callback on error (optional)
 * @returns Promise that resolves with the full summary when complete
 */
export async function streamSummarize(
	transport: Transport,
	options: StreamSummarizeOptions,
	onChunk?: (chunk: StreamSummarizeChunk) => void,
	onError?: (error: Error) => void,
): Promise<StreamSummarizeResult> {
	const client = createFeedClient(transport);

	// Validate options
	if (!options.feedUrl && !options.articleId) {
		throw new Error("Either feedUrl or articleId is required");
	}

	let articleId = "";
	let fullSummary = "";
	let wasCached = false;

	try {
		const stream = client.streamSummarize({
			feedUrl: options.feedUrl,
			articleId: options.articleId,
			content: options.content,
			title: options.title,
		});

		for await (const response of stream) {
			// Update article ID if provided
			if (response.articleId) {
				articleId = response.articleId;
			}

			// Check if cached response
			if (response.isCached && response.fullSummary) {
				wasCached = true;
				fullSummary = response.fullSummary;
			}

			// Accumulate chunks if not cached
			if (!response.isCached && response.chunk) {
				fullSummary += response.chunk;
			}

			// If final message with full summary, use that
			if (response.isFinal && response.fullSummary) {
				fullSummary = response.fullSummary;
			}

			// Call onChunk callback if provided
			if (onChunk) {
				onChunk({
					chunk: response.chunk,
					isFinal: response.isFinal,
					articleId: response.articleId,
					isCached: response.isCached,
					fullSummary: response.fullSummary ?? null,
				});
			}
		}

		return {
			articleId,
			summary: fullSummary,
			wasCached,
		};
	} catch (error) {
		if (onError && error instanceof Error) {
			onError(error);
		}
		throw error;
	}
}

/**
 * Stream article summarization with AbortController support.
 *
 * @param transport - The Connect transport to use
 * @param options - Request options (feedUrl or articleId required)
 * @param onChunk - Callback when a chunk is received
 * @param onComplete - Callback when streaming completes successfully
 * @param onError - Callback on error (optional)
 * @returns AbortController to cancel the stream
 */
export function streamSummarizeWithAbort(
	transport: Transport,
	options: StreamSummarizeOptions,
	onChunk: (chunk: StreamSummarizeChunk) => void,
	onComplete: (result: StreamSummarizeResult) => void,
	onError?: (error: Error) => void,
): AbortController {
	const abortController = new AbortController();

	// Validate options
	if (!options.feedUrl && !options.articleId) {
		const error = new Error("Either feedUrl or articleId is required");
		if (onError) {
			onError(error);
		}
		return abortController;
	}

	const client = createFeedClient(transport);

	// Start streaming in background
	(async () => {
		let articleId = "";
		let fullSummary = "";
		let wasCached = false;

		try {
			const stream = client.streamSummarize(
				{
					feedUrl: options.feedUrl,
					articleId: options.articleId,
					content: options.content,
					title: options.title,
				},
				{ signal: abortController.signal },
			);

			for await (const response of stream) {
				// Update article ID if provided
				if (response.articleId) {
					articleId = response.articleId;
				}

				// Check if cached response
				if (response.isCached && response.fullSummary) {
					wasCached = true;
					fullSummary = response.fullSummary;
				}

				// Accumulate chunks if not cached
				if (!response.isCached && response.chunk) {
					fullSummary += response.chunk;
				}

				// If final message with full summary, use that
				if (response.isFinal && response.fullSummary) {
					fullSummary = response.fullSummary;
				}

				// Call onChunk callback
				onChunk({
					chunk: response.chunk,
					isFinal: response.isFinal,
					articleId: response.articleId,
					isCached: response.isCached,
					fullSummary: response.fullSummary ?? null,
				});
			}

			// Call onComplete when streaming finishes successfully
			onComplete({
				articleId,
				summary: fullSummary,
				wasCached,
			});
		} catch (error) {
			// Only report error if not aborted
			if (!abortController.signal.aborted && onError && error instanceof Error) {
				onError(error);
			}
		}
	})();

	return abortController;
}

// =============================================================================
// Mark As Read Functions (Phase 7)
// =============================================================================

/**
 * Result of marking a feed as read
 */
export interface MarkAsReadResult {
	message: string;
}

/**
 * Normalizes a URL by removing trailing slash (except for root path).
 * This ensures consistent URL comparison across frontend and backend.
 */
function normalizeUrl(url: string): string {
	if (url.endsWith('/') && url !== '/') {
		return url.slice(0, -1);
	}
	return url;
}

/**
 * Marks a feed/article as read via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param feedUrl - The URL of the feed/article to mark as read
 * @returns The success message
 */
export async function markAsRead(
	transport: Transport,
	articleUrl: string,
): Promise<MarkAsReadResult> {
	// Normalize URL to remove trailing slash (zero-trust: ensure consistency)
	const normalizedUrl = normalizeUrl(articleUrl);

	const client = createFeedClient(transport);
	const response = await client.markAsRead({ articleUrl: normalizedUrl });

	return {
		message: response.message,
	};
}
