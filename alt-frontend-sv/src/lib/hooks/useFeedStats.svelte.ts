/**
 * Unified feed stats hook with feature flag support.
 *
 * Automatically switches between SSE and Connect-RPC Streaming
 * based on the USE_CONNECT_STREAMING environment variable.
 */

import { env } from "$env/dynamic/public";
import { useSSEFeedsStats } from "./useSSEFeedsStats.svelte";
import { useStreamingFeedStats } from "./useStreamingFeedStats.svelte";

interface FeedStatsState {
	feedAmount: number;
	unsummarizedArticlesAmount: number;
	totalArticlesAmount: number;
	isConnected: boolean;
	retryCount: number;
}

/**
 * Unified hook for feed statistics.
 *
 * Uses Connect-RPC Streaming if PUBLIC_USE_CONNECT_STREAMING=true,
 * otherwise falls back to SSE.
 *
 * @returns Feed stats state
 */
export function useFeedStats(): FeedStatsState {
	const useStreaming = env.PUBLIC_USE_CONNECT_STREAMING === "true";

	if (useStreaming) {
		console.log("[useFeedStats] Using Connect-RPC Streaming");
		return useStreamingFeedStats();
	} else {
		console.log("[useFeedStats] Using SSE (legacy)");
		return useSSEFeedsStats();
	}
}
