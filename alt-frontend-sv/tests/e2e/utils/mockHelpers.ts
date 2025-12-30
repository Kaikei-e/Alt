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
	const body = chunks.map((chunk) => `event: delta\ndata: ${chunk}\n\n`).join("");
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
