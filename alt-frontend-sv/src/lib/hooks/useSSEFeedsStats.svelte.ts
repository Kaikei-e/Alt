import { onMount } from "svelte";
import { setupSSEWithReconnect } from "$lib/api/sse";
import type { UnsummarizedFeedStatsSummary } from "$lib/api/sse";

// Type guard for validating numeric amounts
const isValidAmount = (value: unknown): value is number => {
	return (
		typeof value === "number" && !isNaN(value) && value >= 0 && isFinite(value)
	);
};

export function useSSEFeedsStats() {
	let feedAmount = $state(0);
	let unsummarizedArticlesAmount = $state(0);
	let totalArticlesAmount = $state(0);
	let isConnected = $state(false);
	let retryCount = $state(0);
	let lastDataReceived = $state(Date.now());

	// Connection health check
	let healthCheckInterval: ReturnType<typeof setInterval> | null = null;
	let currentEventSource: EventSource | null = null;
	let cleanupFn: (() => void) | null = null;
	let isInitialized = false;

	onMount(() => {
		// Prevent multiple initializations
		if (isInitialized) {
			console.warn(`[SSE] useSSEFeedsStats already initialized, skipping`);
			return;
		}
		isInitialized = true;
		// SSE endpoint configuration
		// Use relative path to go through nginx proxy for proper SSE handling
		// nginx has special configuration for /api/v1/sse/ with proxy_buffering off
		const sseUrl = "/api/v1/sse/feeds/stats";

		// Set initial disconnected state
		isConnected = false;
		retryCount = 0;

		const { eventSource: es, cleanup } = setupSSEWithReconnect(
			sseUrl,
			(data: UnsummarizedFeedStatsSummary) => {
				try {
					// Handle feed amount
					if (data.feed_amount?.amount !== undefined) {
						const amount = data.feed_amount.amount;
						if (isValidAmount(amount)) {
							feedAmount = amount;
						} else {
							feedAmount = 0;
						}
					}
				} catch (error) {
					console.error("Error handling feed amount:", error);
				}

				try {
					// Handle unsummarized articles
					if (data.unsummarized_feed?.amount !== undefined) {
						const amount = data.unsummarized_feed.amount;
						if (isValidAmount(amount)) {
							unsummarizedArticlesAmount = amount;
						} else {
							unsummarizedArticlesAmount = 0;
						}
					}
				} catch (error) {
					console.error("Error handling unsummarized articles:", error);
				}

				try {
					// Handle total articles
					const totalArticlesAmountValue = data.total_articles?.amount ?? 0;
					if (isValidAmount(totalArticlesAmountValue)) {
						totalArticlesAmount = totalArticlesAmountValue;
					} else {
						totalArticlesAmount = 0;
					}
				} catch (error) {
					console.error("Error handling total articles:", error);
				}

				// Update connection state and reset retry count on successful data
				const now = Date.now();
				lastDataReceived = now;

				isConnected = true;
				retryCount = 0;
				console.log(`[SSE] Data received, updating connection state`, {
					lastDataReceived: now,
					isConnected: true,
					// Verify state after assignment
					actualIsConnected: isConnected,
				});
			},
			() => {
				// Handle SSE connection error
				console.warn(`[SSE] Connection error occurred`, {
					retryCount: retryCount + 1,
					readyState: currentEventSource?.readyState,
				});
				isConnected = false;
				retryCount++;
			},
			3, // Max 3 reconnect attempts
			() => {
				// Handle SSE connection opened
				const now = Date.now();
				lastDataReceived = now;
				isConnected = true;
				retryCount = 0;
				console.log(`[SSE] Connection opened successfully`, {
					lastDataReceived: now,
					isConnected: true,
					// Verify state after assignment
					actualIsConnected: isConnected,
				});
			},
			() => {
				// Handle heartbeat - update lastDataReceived to keep connection state healthy
				const now = Date.now();
				lastDataReceived = now;
				console.log(`[SSE] Heartbeat received, updating lastDataReceived`, {
					lastDataReceived: now,
				});
				// Don't change isConnected here - let health check handle it
				// But update timestamp so health check knows connection is alive
			},
		);

		currentEventSource = es;

		// Connection health check
		// Backend sends heartbeat every 10s and data updates periodically
		// Use 25s timeout (2.5x heartbeat interval) to account for network delays
		const DATA_TIMEOUT_MS = 25000; // 25 seconds - backend heartbeat is 10s
		const HEALTH_CHECK_INTERVAL_MS = 5000; // Check every 5 seconds

		healthCheckInterval = setInterval(() => {
			const now = Date.now();
			const timeSinceLastData = now - lastDataReceived;
			const readyState = currentEventSource?.readyState ?? EventSource.CLOSED;

			// Check if we're receiving data regularly
			// Backend sends heartbeat every 10s, so we should receive something within 25s
			const isReceivingData = timeSinceLastData < DATA_TIMEOUT_MS;
			const isConnectionOpen = readyState === EventSource.OPEN;

			// Connection is healthy if:
			// 1. Connection is open (readyState === OPEN)
			// 2. We've received data recently (within timeout)
			const shouldBeConnected = isConnectionOpen && isReceivingData;

			// Debug log every health check (use console.log for visibility)
			console.log(`[SSE] Health check`, {
				readyState: readyState === EventSource.OPEN ? "OPEN" : readyState === EventSource.CONNECTING ? "CONNECTING" : "CLOSED",
				timeSinceLastData: `${timeSinceLastData}ms`,
				isReceivingData,
				isConnectionOpen,
				shouldBeConnected,
				currentIsConnected: isConnected,
			});

			// Log state changes for debugging
			if (isConnected !== shouldBeConnected) {
				if (shouldBeConnected) {
					console.log(`[SSE] Connection state: CONNECTED`, {
						readyState: readyState === EventSource.OPEN ? "OPEN" : "CLOSED",
						timeSinceLastData: `${timeSinceLastData}ms`,
					});
				} else {
					console.warn(`[SSE] Connection state: DISCONNECTED`, {
						readyState: readyState === EventSource.OPEN ? "OPEN" : "CLOSED",
						timeSinceLastData: `${timeSinceLastData}ms`,
						timeout: `${DATA_TIMEOUT_MS}ms`,
					});
				}
				isConnected = shouldBeConnected;
			}

			// Warn if connection is open but we haven't received data in a while
			if (isConnectionOpen && !isReceivingData && timeSinceLastData > DATA_TIMEOUT_MS) {
				console.warn(`[SSE] Connection open but no data received`, {
					timeSinceLastData: `${timeSinceLastData}ms`,
					timeout: `${DATA_TIMEOUT_MS}ms`,
				});
			}
		}, HEALTH_CHECK_INTERVAL_MS);

		cleanupFn = () => {
			console.log(`[SSE] Cleaning up useSSEFeedsStats`);
			if (healthCheckInterval) {
				clearInterval(healthCheckInterval);
				healthCheckInterval = null;
			}
			if (cleanup) {
				cleanup();
			}
			isInitialized = false;
		};

		return cleanupFn;
	});

	// Debug: Log when isConnected changes
	$effect(() => {
		console.log(`[Hook] isConnected state changed:`, isConnected);
	});

	// Use $derived to ensure reactivity is preserved when returning from function
	// This ensures that changes to isConnected are tracked by consumers
	const derivedIsConnected = $derived(isConnected);
	const derivedRetryCount = $derived(retryCount);
	const derivedFeedAmount = $derived(feedAmount);
	const derivedUnsummarizedArticlesAmount = $derived(unsummarizedArticlesAmount);
	const derivedTotalArticlesAmount = $derived(totalArticlesAmount);

	return {
		get feedAmount() {
			return derivedFeedAmount;
		},
		get unsummarizedArticlesAmount() {
			return derivedUnsummarizedArticlesAmount;
		},
		get totalArticlesAmount() {
			return derivedTotalArticlesAmount;
		},
		get isConnected() {
			return derivedIsConnected;
		},
		get retryCount() {
			return derivedRetryCount;
		},
	};
}

