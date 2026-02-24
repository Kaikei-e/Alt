/**
 * API Mock utilities for E2E tests
 * Re-exports from the new playwright-msw module for backward compatibility
 */

export {
  type MockOptions,
  mockArticleDetailApi,
  mockErrorResponse,
  mockFeedsApi,
  mockSearchApi,
  setupPlaywrightMocks as setupAllMocks,
  setupPlaywrightMocks as setupFeedMocks,
} from "../mocks/playwright-msw";
