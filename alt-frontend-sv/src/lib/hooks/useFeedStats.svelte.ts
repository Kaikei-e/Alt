/**
 * Unified feed stats hook backed by Connect-RPC Server Streaming.
 *
 * The previous SSE fallback was retired together with alt-backend's
 * /v1/sse/feeds/stats endpoint (H-001) because Connect-RPC streaming is now
 * the only authenticated path. The hook keeps the same public shape so
 * existing components do not need to change.
 */

import { useStreamingFeedStats } from "./useStreamingFeedStats.svelte";

interface FeedStatsState {
	feedAmount: number;
	unsummarizedArticlesAmount: number;
	totalArticlesAmount: number;
	isConnected: boolean;
	retryCount: number;
	reconnect: () => void;
}

export function useFeedStats(): FeedStatsState {
	return useStreamingFeedStats();
}
