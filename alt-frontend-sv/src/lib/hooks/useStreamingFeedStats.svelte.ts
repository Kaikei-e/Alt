/**
 * Svelte 5 hook for streaming feed statistics via Connect-RPC Server Streaming.
 *
 * The hook is backed by a module-scope singleton so multiple components on the
 * same page (or rapid SvelteKit navigations) reuse a single stream rather than
 * opening one per mount. Production data showed 22+ concurrent streams from a
 * single user as components mounted and re-mounted; the singleton + refcount
 * pattern collapses those to a single stream while still supporting cleanup
 * on the last subscriber unmount.
 */

import { onDestroy, untrack } from "svelte";
import { createClientTransport } from "$lib/connect/transport.client";
import { streamFeedStats } from "$lib/connect/feeds";

interface StreamingFeedStatsState {
	feedAmount: number;
	unsummarizedArticlesAmount: number;
	totalArticlesAmount: number;
	isConnected: boolean;
	retryCount: number;
	reconnect: () => void;
}

const MAX_RETRIES = 5;
const BASE_RETRY_DELAY = 1000; // 1 second
const MAX_RETRY_DELAY = 30_000; // 30 seconds

// =============================================================================
// Module-scope singleton state
// =============================================================================

let feedAmount = $state(0);
let unsummarizedArticlesAmount = $state(0);
let totalArticlesAmount = $state(0);
let isConnected = $state(false);
let retryCount = $state(0);

let abortController: AbortController | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let subscriberCount = 0;
let visibilityListenersAttached = false;
let suspendedForHidden = false;

function connect(): void {
	// Only connect when at least one subscriber is active.
	if (subscriberCount === 0) return;
	if (abortController) return;
	if (untrack(() => retryCount) >= MAX_RETRIES) {
		console.error("[useStreamingFeedStats] Max retry attempts reached");
		return;
	}

	void (async () => {
		try {
			const transport = createClientTransport();
			abortController = await streamFeedStats(
				transport,
				(stats) => {
					if (!stats.isHeartbeat) {
						feedAmount = stats.feedAmount;
						unsummarizedArticlesAmount = stats.unsummarizedFeedAmount;
						totalArticlesAmount = stats.totalArticles;
					}
					isConnected = true;
					retryCount = 0;
				},
				(error) => {
					console.error("[useStreamingFeedStats] Stream error:", error);
					isConnected = false;
					abortController = null;
					if (subscriberCount > 0 && !suspendedForHidden) {
						scheduleReconnect();
					}
				},
			);
			isConnected = true;
		} catch (error) {
			console.error("[useStreamingFeedStats] Connection failed:", error);
			isConnected = false;
			abortController = null;
			if (subscriberCount > 0 && !suspendedForHidden) {
				scheduleReconnect();
			}
		}
	})();
}

function scheduleReconnect(): void {
	if (reconnectTimer) return;
	const attempt = untrack(() => retryCount);
	const delay = Math.min(BASE_RETRY_DELAY * 2 ** attempt, MAX_RETRY_DELAY);
	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		retryCount = attempt + 1;
		connect();
	}, delay);
}

function disconnect(): void {
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

function suspend(): void {
	if (suspendedForHidden) return;
	suspendedForHidden = true;
	disconnect();
}

function resume(): void {
	if (!suspendedForHidden) return;
	suspendedForHidden = false;
	if (subscriberCount > 0) {
		retryCount = 0;
		connect();
	}
}

function attachVisibilityListenersOnce(): void {
	if (visibilityListenersAttached) return;
	if (typeof document === "undefined") return;
	visibilityListenersAttached = true;
	document.addEventListener("visibilitychange", () => {
		if (document.visibilityState === "hidden") {
			suspend();
		} else {
			resume();
		}
	});
	// pagehide fires when the page enters BFCache / before unload; pause to
	// avoid stranded streams on the server.
	window.addEventListener("pagehide", () => {
		suspend();
	});
	window.addEventListener("pageshow", (e) => {
		// pageshow with persisted=true means BFCache restore.
		if ((e as PageTransitionEvent).persisted) {
			resume();
		}
	});
}

// =============================================================================
// Public hook
// =============================================================================

/**
 * Hook for streaming feed statistics via Connect-RPC.
 *
 * Returns reactive state with getter pattern for Svelte 5 compatibility.
 * Multiple consumers across the page share a single underlying stream.
 */
export function useStreamingFeedStats(): StreamingFeedStatsState {
	attachVisibilityListenersOnce();

	subscriberCount += 1;
	if (subscriberCount === 1) {
		retryCount = 0;
		connect();
	}

	onDestroy(() => {
		subscriberCount = Math.max(0, subscriberCount - 1);
		if (subscriberCount === 0) {
			disconnect();
		}
	});

	function reconnect(): void {
		retryCount = 0;
		disconnect();
		if (subscriberCount > 0) {
			connect();
		}
	}

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
		reconnect,
	};
}
