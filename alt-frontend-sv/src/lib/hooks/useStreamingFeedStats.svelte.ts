/**
 * Svelte 5 hook for streaming feed statistics via Connect-RPC Server Streaming.
 *
 * Replaces useSSEFeedsStats with type-safe gRPC streaming.
 */

import { onDestroy } from "svelte";
import { createClientTransport } from "$lib/connect/transport";
import { streamFeedStats } from "$lib/connect/feeds";

interface StreamingFeedStatsState {
	feedAmount: number;
	unsummarizedArticlesAmount: number;
	totalArticlesAmount: number;
	isConnected: boolean;
	retryCount: number;
}

const MAX_RETRIES = 3;
const BASE_RETRY_DELAY = 1000; // 1 second
const MAX_RETRY_DELAY = 10000; // 10 seconds

/**
 * Hook for streaming feed statistics via Connect-RPC.
 *
 * Returns reactive state with getter pattern for Svelte 5 compatibility.
 *
 * @returns Streaming feed stats state
 */
export function useStreamingFeedStats(): StreamingFeedStatsState {
	// Reactive state with $state runes
	let feedAmount = $state(0);
	let unsummarizedArticlesAmount = $state(0);
	let totalArticlesAmount = $state(0);
	let isConnected = $state(false);
	let retryCount = $state(0);

	let abortController: AbortController | null = null;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

	/**
	 * Establish streaming connection
	 */
	async function connect() {
		if (retryCount >= MAX_RETRIES) {
			console.error("[useStreamingFeedStats] Max retry attempts reached");
			return;
		}

		try {
			const transport = createClientTransport();

			abortController = await streamFeedStats(
				transport,
				(stats) => {
					// Ignore heartbeat messages for data updates
					if (!stats.isHeartbeat) {
						feedAmount = stats.feedAmount;
						unsummarizedArticlesAmount = stats.unsummarizedFeedAmount;
						totalArticlesAmount = stats.totalArticles;
					}

					// Mark as connected (even for heartbeats)
					isConnected = true;
					retryCount = 0; // Reset on successful data
				},
				(error) => {
					console.error("[useStreamingFeedStats] Stream error:", error);
					isConnected = false;
					scheduleReconnect();
				},
			);

			isConnected = true;
		} catch (error) {
			console.error("[useStreamingFeedStats] Connection failed:", error);
			isConnected = false;
			scheduleReconnect();
		}
	}

	/**
	 * Schedule reconnection with exponential backoff
	 */
	function scheduleReconnect() {
		if (reconnectTimer) return; // Already scheduled

		const delay = Math.min(
			BASE_RETRY_DELAY * Math.pow(2, retryCount),
			MAX_RETRY_DELAY,
		);

		console.log(
			`[useStreamingFeedStats] Scheduling reconnect in ${delay}ms (attempt ${retryCount + 1}/${MAX_RETRIES})`,
		);

		reconnectTimer = setTimeout(() => {
			reconnectTimer = null;
			retryCount++;
			connect();
		}, delay);
	}

	/**
	 * Disconnect and cleanup
	 */
	function disconnect() {
		if (abortController) {
			abortController.abort();
			abortController = null;
		}

		if (reconnectTimer) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}

		isConnected = false;
	}

	// Initial connection
	connect();

	// Cleanup on component destroy
	onDestroy(() => {
		disconnect();
	});

	// Return reactive getters (preserves Svelte 5 reactivity)
	return {
		get feedAmount() {
			return feedAmount;
		},
		get unsummarizedArticlesAmount() {
			return unsummarizedArticlesAmount;
		},
		get totalArticlesAmount() {
			return totalArticlesAmount;
		},
		get isConnected() {
			return isConnected;
		},
		get retryCount() {
			return retryCount;
		},
	};
}
