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

	onMount(() => {
		// SSE endpoint configuration
		const apiBaseUrl = import.meta.env.PUBLIC_API_BASE_URL || "http://localhost:9000";
		const sseUrl = `${apiBaseUrl}/v1/sse/feeds/stats`;

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
			},
			() => {
				// Handle SSE connection error
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
			},
		);

		currentEventSource = es;

		// Connection health check
		healthCheckInterval = setInterval(() => {
			const now = Date.now();
			const timeSinceLastData = now - lastDataReceived;
			const readyState = currentEventSource?.readyState ?? EventSource.CLOSED;

			// Backend sends data every 5s, so 15s timeout gives buffer for network delays
			const isReceivingData = timeSinceLastData < 15000; // 15s timeout (3x backend interval)
			const isConnectionOpen = readyState === EventSource.OPEN;

			// Connection is healthy if open AND receiving data regularly
			const shouldBeConnected = isConnectionOpen && isReceivingData;

			// Only update state if it actually changed to prevent unnecessary re-renders
			if (isConnected !== shouldBeConnected) {
				isConnected = shouldBeConnected;
			}
		}, 5000); // Check every 5 seconds to reduce overhead

		return () => {
			if (healthCheckInterval) {
				clearInterval(healthCheckInterval);
			}
			cleanup();
		};
	});

	return {
		feedAmount,
		unsummarizedArticlesAmount,
		totalArticlesAmount,
		isConnected,
		retryCount,
	};
}

