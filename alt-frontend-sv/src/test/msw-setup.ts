/**
 * MSW setup for Vitest component tests.
 *
 * Reuses the same handlers defined for E2E tests, ensuring mock data
 * consistency across unit/component and E2E test layers.
 *
 * Usage in vitest tests:
 *   import { server } from '$test/msw-setup'
 *   import { beforeAll, afterEach, afterAll } from 'vitest'
 *
 *   beforeAll(() => server.listen({ onUnhandledRequest: 'warn' }))
 *   afterEach(() => server.resetHandlers())
 *   afterAll(() => server.close())
 */

import { setupServer } from "msw/node";
import { handlers } from "../../tests/e2e/infra/msw-handlers";

export const server = setupServer(...handlers);

export function addHandlers(
	...newHandlers: Parameters<typeof server.use>
): void {
	server.use(...newHandlers);
}

export { http, HttpResponse } from "msw";
export { handlers } from "../../tests/e2e/infra/msw-handlers";
