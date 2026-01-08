/**
 * MSW Server for E2E Tests
 *
 * This module provides MSW's setupServer for use in tests.
 * It uses the same mock data as the http.createServer implementation,
 * but leverages MSW's request interception for more realistic testing.
 *
 * Usage in tests:
 *   import { server } from './msw-server'
 *   import { beforeAll, afterEach, afterAll } from 'vitest'
 *
 *   beforeAll(() => server.listen({ onUnhandledRequest: 'warn' }))
 *   afterEach(() => server.resetHandlers())
 *   afterAll(() => server.close())
 */

import { setupServer } from "msw/node";
import { handlers } from "./msw-handlers";

/**
 * MSW server instance with all mock handlers
 */
export const server = setupServer(...handlers);

/**
 * Add custom handlers for specific test scenarios
 *
 * @example
 * import { server, addHandlers } from './msw-server'
 * import { http, HttpResponse } from 'msw'
 *
 * test('error scenario', () => {
 *   addHandlers(
 *     http.get('* /v1/feeds/fetch/cursor', () =>
 *       HttpResponse.json({ error: 'Something went wrong' }, { status: 500 })
 *     )
 *   )
 *   // ... test error handling
 * })
 */
export function addHandlers(...newHandlers: Parameters<typeof server.use>) {
	server.use(...newHandlers);
}

// Re-export handlers for custom composition
export { handlers } from "./msw-handlers";

// Re-export MSW utilities for convenience
export { http, HttpResponse } from "msw";
