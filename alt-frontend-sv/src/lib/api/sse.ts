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
	let lastDataReceivedTime = 0; // Track when we last received data
	let connectionStartTime = Date.now(); // Track connection start time
	const reconnectDelays: number[] = []; // Track reconnect delays for monitoring

	const connect = () => {
		try {
			// Close existing connection if any
			if (eventSource) {
				eventSource.close();
				eventSource = null;
			}

			const attemptStartTime = Date.now();
			console.log(`[SSE] Attempting to connect (attempt ${reconnectAttempts + 1}/${maxReconnectAttempts + 1})`, {
				endpoint,
				reconnectAttempts,
			});

			eventSource = new EventSource(endpoint);

			eventSource.onopen = () => {
				const connectionTime = Date.now() - attemptStartTime;
				console.log(`[SSE] Connection opened`, {
					endpoint,
					connectionTime: `${connectionTime}ms`,
					readyState: eventSource?.readyState,
				});

				// Reset reconnect attempts on successful open
				if (reconnectAttempts > 0) {
					console.log(`[SSE] Connection restored after ${reconnectAttempts} reconnection attempts`);
					reconnectAttempts = 0;
					reconnectDelays.length = 0; // Clear delay history
				}

				if (onOpen) {
					onOpen();
				}
			};

			eventSource.onmessage = (event) => {
				try {
					// Ignore heartbeat comments
					if (event.data.trim().startsWith(":")) {
						return;
					}

					const data = JSON.parse(event.data) as UnsummarizedFeedStatsSummary;
					// Validate basic structure before passing to callback
					if (data && typeof data === "object") {
						// Only reset attempts when we successfully receive and parse data
						if (!hasReceivedData) {
							hasReceivedData = true;
							const timeToFirstData = Date.now() - connectionStartTime;
							console.log(`[SSE] First data received`, {
								endpoint,
								timeToFirstData: `${timeToFirstData}ms`,
							});
						}
						lastDataReceivedTime = Date.now();
						reconnectAttempts = 0; // Reset only on successful data reception
						reconnectDelays.length = 0; // Clear delay history on successful data
						onData(data);
					}
				} catch (error) {
					console.error("[SSE] Error parsing SSE message:", {
						error,
						data: event.data?.substring(0, 100), // Log first 100 chars
					});
				}
			};

			eventSource.onerror = (error) => {
				const readyState = eventSource?.readyState ?? EventSource.CLOSED;
				const states = ["CONNECTING", "OPEN", "CLOSED"];
				const stateName = states[readyState] ?? "UNKNOWN";

				console.warn(`[SSE] Connection error`, {
					endpoint,
					readyState: stateName,
					reconnectAttempts,
					maxReconnectAttempts,
					hasReceivedData,
					timeSinceLastData: lastDataReceivedTime
						? `${Date.now() - lastDataReceivedTime}ms`
						: "never",
				});

				if (onError) {
					onError();
				}

				// Check if we should reconnect
				// Only reconnect if:
				// 1. We haven't exceeded max attempts
				// 2. Connection is actually closed (not just a temporary error)
				const shouldReconnect =
					reconnectAttempts < maxReconnectAttempts && readyState === EventSource.CLOSED;

				if (shouldReconnect) {
					// Close current connection
					eventSource?.close();

					reconnectAttempts++;
					// Exponential backoff with jitter: base delay * 2^(attempt-1) + random(0-1000ms)
					const baseDelay = Math.min(2 ** (reconnectAttempts - 1) * 1000, 10000);
					const jitter = Math.floor(Math.random() * 1000);
					const delay = baseDelay + jitter;
					reconnectDelays.push(delay);

					console.log(`[SSE] Scheduling reconnection`, {
						endpoint,
						attempt: reconnectAttempts + 1,
						delay: `${delay}ms`,
						delays: reconnectDelays,
					});

					reconnectTimeout = setTimeout(() => {
						connect();
					}, delay);
				} else {
					// Max attempts reached or connection not closed
					if (reconnectAttempts >= maxReconnectAttempts) {
						console.error(`[SSE] Max reconnection attempts reached`, {
							endpoint,
							attempts: reconnectAttempts,
							delays: reconnectDelays,
						});
					}
					eventSource?.close();
				}
			};
		} catch (error) {
			console.error("[SSE] Error creating SSE connection:", {
				error,
				endpoint,
				reconnectAttempts,
			});

			if (onError) {
				onError();
			}

			// Try to reconnect if we haven't exceeded max attempts
			if (reconnectAttempts < maxReconnectAttempts) {
				reconnectAttempts++;
				const delay = Math.min(2 ** (reconnectAttempts - 1) * 1000, 10000);
				reconnectTimeout = setTimeout(() => {
					connect();
				}, delay);
			}
		}
	};

	const cleanup = () => {
		console.log(`[SSE] Cleaning up connection`, {
			endpoint,
			reconnectAttempts,
			hasReceivedData,
		});

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

