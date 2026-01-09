import { setupServer } from 'msw/node';
import { handlers } from './handlers';

/**
 * MSW server instance for E2E tests
 * This intercepts network requests at the Node.js level
 */
export const server = setupServer(...handlers);

/**
 * Start the MSW server with default handlers
 */
export function startMswServer() {
  server.listen({
    onUnhandledRequest: 'bypass', // Don't warn about unhandled requests
  });
  console.log('[MSW] Server started');
}

/**
 * Stop the MSW server
 */
export function stopMswServer() {
  server.close();
  console.log('[MSW] Server stopped');
}

/**
 * Reset handlers to defaults (useful between tests)
 */
export function resetMswHandlers() {
  server.resetHandlers();
}
