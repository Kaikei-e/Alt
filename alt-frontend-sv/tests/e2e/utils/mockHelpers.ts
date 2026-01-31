import type { Route } from "@playwright/test";

/**
 * Fulfill a route with JSON response.
 * Consolidates duplicated fulfillJson helper from individual test files.
 */
export async function fulfillJson(
	route: Route,
	body: unknown,
	status = 200,
): Promise<void> {
	await route.fulfill({
		status,
		contentType: "application/json",
		body: JSON.stringify(body),
	});
}

/**
 * Fulfill a route with Connect-RPC streaming response.
 * Connect-RPC uses a binary envelope format for streaming responses:
 * - 1 byte: flags (0x00 for data messages, 0x02 for end-of-stream trailer)
 * - 4 bytes: message length (big-endian)
 * - N bytes: JSON-encoded message payload
 */
export async function fulfillConnectStream(
	route: Route,
	messages: unknown[],
	status = 200,
): Promise<void> {
	// Build binary envelope for each message
	const buffers: Buffer[] = [];

	for (const msg of messages) {
		const jsonPayload = JSON.stringify(msg);
		const payloadBytes = Buffer.from(jsonPayload, "utf-8");

		// Create envelope: flags (1 byte) + length (4 bytes big-endian) + payload
		const envelope = Buffer.alloc(5 + payloadBytes.length);
		envelope[0] = 0x00; // flags: data message
		envelope.writeUInt32BE(payloadBytes.length, 1); // length
		payloadBytes.copy(envelope, 5); // payload

		buffers.push(envelope);
	}

	// Add end-of-stream trailer (flags = 0x02 with empty JSON object)
	const trailerPayload = Buffer.from("{}", "utf-8");
	const trailer = Buffer.alloc(5 + trailerPayload.length);
	trailer[0] = 0x02; // flags: end-of-stream trailer
	trailer.writeUInt32BE(trailerPayload.length, 1);
	trailerPayload.copy(trailer, 5);
	buffers.push(trailer);

	const body = Buffer.concat(buffers);

	await route.fulfill({
		status,
		contentType: "application/connect+json",
		headers: {
			"Connect-Content-Encoding": "identity",
			"Connect-Accept-Encoding": "identity",
		},
		body,
	});
}

/**
 * Fulfill a route with a streaming (SSE) response.
 * Useful for mocking Augur chat and other streaming endpoints.
 *
 * For Augur, the streaming format uses `event: delta` for text chunks.
 * Each chunk should be a JSON string with a "text" field.
 */
export async function fulfillStream(
	route: Route,
	chunks: string[],
	status = 200,
): Promise<void> {
	// Build proper SSE format with event types
	const body = chunks
		.map((chunk) => `event: delta\ndata: ${chunk}\n\n`)
		.join("");
	await route.fulfill({
		status,
		contentType: "text/event-stream",
		headers: {
			"Cache-Control": "no-cache",
			Connection: "keep-alive",
		},
		body: `${body}event: done\ndata: {}\n\n`,
	});
}

/**
 * Fulfill a route with an error response.
 */
export async function fulfillError(
	route: Route,
	message: string,
	status = 500,
): Promise<void> {
	await route.fulfill({
		status,
		contentType: "application/json",
		body: JSON.stringify({ error: message }),
	});
}

/**
 * Create a mock EventSource class for browser injection.
 * Useful for testing pages that use EventSource (SSE).
 */
export function createMockEventSourceScript(): string {
	return `
    class MockEventSource {
      url;
      withCredentials = false;
      readyState = 1;

      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSED = 2;

      CONNECTING = 0;
      OPEN = 1;
      CLOSED = 2;

      onopen = null;
      onmessage = null;
      onerror = null;

      constructor(url) {
        this.url = url;
        setTimeout(() => {
          this.onopen?.(new Event("open"));
        }, 0);
      }

      close() {
        this.readyState = 2;
      }

      addEventListener() {}
      removeEventListener() {}
      dispatchEvent() {
        return false;
      }
    }

    window.EventSource = MockEventSource;
  `;
}
