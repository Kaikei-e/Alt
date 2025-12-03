import { browser } from "$app/environment";

export interface UnsummarizedFeedStatsSummary {
	feed_amount: { amount: number };
	unsummarized_feed: { amount: number };
	total_articles?: { amount: number };
}

export function setupSSE(
	endpoint: string,
	onData: (data: UnsummarizedFeedStatsSummary) => void,
	onError?: () => void,
): EventSource | null {
	if (!browser) {
		return null;
	}

	try {
		const eventSource = new EventSource(endpoint);

		eventSource.onmessage = (event) => {
			try {
				const data = JSON.parse(event.data) as UnsummarizedFeedStatsSummary;
				// Validate basic structure before passing to callback
				if (data && typeof data === "object") {
					onData(data);
				}
			} catch (error) {
				console.error("Error parsing SSE message:", error);
			}
		};

		eventSource.onerror = () => {
			if (onError) {
				onError();
			}
		};

		return eventSource;
	} catch {
		console.error("Error creating SSE connection");
		if (onError) {
			onError();
		}
		return null;
	}
}

export function setupSSEWithReconnect(
	endpoint: string,
	onData: (data: UnsummarizedFeedStatsSummary) => void,
	onError?: () => void,
	maxReconnectAttempts: number = 3,
	onOpen?: () => void,
): { eventSource: EventSource | null; cleanup: () => void } {
	if (!browser) {
		return { eventSource: null, cleanup: () => {} };
	}

	let eventSource: EventSource | null = null;
	let reconnectAttempts = 0;
	let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
	let hasReceivedData = false; // Track if we've actually received data

	const connect = () => {
		try {
			eventSource = new EventSource(endpoint);

			eventSource.onopen = () => {
				// Don't reset attempts here - only reset when we actually receive data
				if (onOpen) {
					onOpen();
				}
			};

			eventSource.onmessage = (event) => {
				try {
					const data = JSON.parse(event.data) as UnsummarizedFeedStatsSummary;
					// Validate basic structure before passing to callback
					if (data && typeof data === "object") {
						// Only reset attempts when we successfully receive and parse data
						if (!hasReceivedData) {
							hasReceivedData = true;
						}
						reconnectAttempts = 0; // Reset only on successful data reception
						onData(data);
					}
				} catch (error) {
					console.error("Error parsing SSE message:", error);
				}
			};

			eventSource.onerror = () => {
				if (onError) {
					onError();
				}

				// Only close and reconnect if we haven't received data recently
				// This prevents unnecessary reconnections due to temporary network issues
				eventSource?.close();

				if (reconnectAttempts < maxReconnectAttempts) {
					reconnectAttempts++;
					const delay = Math.min(2 ** (reconnectAttempts - 1) * 1000, 10000); // Exponential backoff with max 10s
					reconnectTimeout = setTimeout(connect, delay);
				}
			};
		} catch (error) {
			console.error("Error creating SSE connection:", error);
			if (onError) {
				onError();
			}
		}
	};

	const cleanup = () => {
		if (reconnectTimeout) {
			clearTimeout(reconnectTimeout);
			reconnectTimeout = null;
		}
		if (eventSource) {
			eventSource.close();
			eventSource = null;
		}
	};

	connect();

	return { eventSource, cleanup };
}

