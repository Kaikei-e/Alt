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
import { streamFeedStats } from "#lib/connect/feeds.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import { browser } from "$app/env";

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
let connectInFlight = false;
let connectionGeneration = 0;

function connect(): void {
	// This module holds shared, request-independent singleton state (see file
	// header). It must never mutate on the server: a Node process serves many
	// concurrent SSR requests through the same module instance, so writing
	// stream data here would leak one user's feed stats into another user's
	// response.
	if (!browser) return;
	// Only connect when at least one subscriber is active and tab is not suspended.
	if (subscriberCount === 0 || suspendedForHidden) return;
	if (abortController || connectInFlight) return;
	if (untrack(() => retryCount) >= MAX_RETRIES) {
		console.error("[useStreamingFeedStats] Max retry attempts reached");
		return;
	}

	const generation = ++connectionGeneration;
	connectInFlight = true;

	void (async () => {
		try {
			const transport = createClientTransport();
			const controller = await streamFeedStats(
				transport,
				(stats) => {
					if (generation !== connectionGeneration) return;
					if (!stats.isHeartbeat) {
						feedAmount = stats.feedAmount;
						unsummarizedArticlesAmount = stats.unsummarizedFeedAmount;
						totalArticlesAmount = stats.totalArticles;
					}
					isConnected = true;
					retryCount = 0;
				},
				(error) => {
					if (generation !== connectionGeneration) return;
					console.error("[useStreamingFeedStats] Stream error:", error);
					isConnected = false;
					abortController = null;
					if (subscriberCount > 0 && !suspendedForHidden) {
						scheduleReconnect();
					}
				},
			);
			connectInFlight = false;
			if (
				generation !== connectionGeneration ||
				subscriberCount === 0 ||
				suspendedForHidden
			) {
				controller.abort();
				return;
			}
			abortController = controller;
			isConnected = true;
		} catch (error) {
			connectInFlight = false;
			if (generation !== connectionGeneration) return;
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
	connectionGeneration += 1;
	connectInFlight = false;
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
	document.addEventListener("visibilitychange", onVisibilityChange);
	// pagehide fires when the page enters BFCache / before unload; pause to
	// avoid stranded streams on the server.
	window.addEventListener("pagehide", onPageHide);
	window.addEventListener("pageshow", onPageShow);
}

function onVisibilityChange(): void {
	if (document.visibilityState === "hidden") {
		suspend();
	} else {
		resume();
	}
}

function onPageHide(): void {
	suspend();
}

function onPageShow(e: Event): void {
	// pageshow with persisted=true means BFCache restore.
	if ((e as PageTransitionEvent).persisted) {
		resume();
	}
}

function detachVisibilityListeners(): void {
	if (!visibilityListenersAttached) return;
	if (typeof document === "undefined") return;
	document.removeEventListener("visibilitychange", onVisibilityChange);
	window.removeEventListener("pagehide", onPageHide);
	window.removeEventListener("pageshow", onPageShow);
	visibilityListenersAttached = false;
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
			suspendedForHidden = false;
			disconnect();
			detachVisibilityListeners();
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
