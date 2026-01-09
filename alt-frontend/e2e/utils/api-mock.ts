/**
 * API Mock utilities for E2E tests
 * Re-exports from the new playwright-msw module for backward compatibility
 */

export {
  setupPlaywrightMocks as setupAllMocks,
  setupPlaywrightMocks as setupFeedMocks,
  mockFeedsApi,
  mockArticleDetailApi,
  mockSearchApi,
  mockErrorResponse,
  type MockOptions,
} from '../mocks/playwright-msw';
