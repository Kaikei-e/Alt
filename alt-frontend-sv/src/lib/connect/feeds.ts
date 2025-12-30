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
