/**
 * Unified feed stats hook with feature flag support.
 *
 * Automatically switches between SSE and Connect-RPC Streaming
 * based on the USE_CONNECT_STREAMING environment variable.
 */

import { useSSEFeedsStats } from "./useSSEFeedsStats.svelte";
import { useStreamingFeedStats } from "./useStreamingFeedStats.svelte";

interface FeedStatsState {
	feedAmount: number;
	unsummarizedArticlesAmount: number;
	totalArticlesAmount: number;
	isConnected: boolean;
	retryCount: number;
	reconnect: () => void;
}

/**
 * Safely read PUBLIC_USE_CONNECT_STREAMING.
 *
 * $env/dynamic/public compiles to `globalThis.__sveltekit_<hash>.env` which
 * throws if the SvelteKit bootstrap hasn't run yet. We lazy-import to avoid
 * crashing at module-evaluation time.
 */
async function getUseStreaming(): Promise<boolean> {
	try {
		const { env } = await import("$env/dynamic/public");
		return env?.PUBLIC_USE_CONNECT_STREAMING === "true";
	} catch {
		return false;
	}
}

// Cache the resolved value so the hook stays synchronous after first call
let _streamingResolved = false;
let _useStreaming = false;
getUseStreaming().then((v) => {
	_useStreaming = v;
	_streamingResolved = true;
});

/**
 * Unified hook for feed statistics.
 *
 * Uses Connect-RPC Streaming if PUBLIC_USE_CONNECT_STREAMING=true,
 * otherwise falls back to SSE.
 *
 * @returns Feed stats state
 */
export function useFeedStats(): FeedStatsState {
	if (_streamingResolved && _useStreaming) {
		return useStreamingFeedStats();
	}
	return useSSEFeedsStats();
}
