import type { Page, Route } from "@playwright/test";

/**
 * Centralized API mock data and helpers for Playwright tests
 * This file contains all mock responses and route patterns for backend APIs
 */

// Mock data types
interface MockFeedStats {
  totalFeeds: number;
  totalArticles: number;
  unreadCount: number;
  dailyReadCount: number;
  feed_amount?: { amount: number };
  summarized_feed?: { amount: number };
}

interface MockFeed {
  id: string;
  title: string;
  description: string;
  link: string;
  published: string;
  unreadCount?: number;
}

interface MockFeedResponse {
  data: MockFeed[];
  next_cursor: string | null;
}

// Mock data constants
export const MOCK_FEED_STATS: MockFeedStats = {
  totalFeeds: 42,
  totalArticles: 1337,
  unreadCount: 15,
  dailyReadCount: 23,
  feed_amount: { amount: 86 },
  summarized_feed: { amount: 50 },
};

export const MOCK_FEEDS: MockFeed[] = Array.from({ length: 10 }, (_, i) => ({
  id: `feed-${i}`,
  title: `Test Feed ${i}`,
  description: `Description for test feed ${i}`,
  link: `https://example.com/feed-${i}`,
  published: new Date().toISOString(),
  unreadCount: Math.floor(Math.random() * 10),
}));

export const MOCK_UNREAD_COUNT = { count: 86 };

export const MOCK_FEED_TAGS = [
  { id: 1, name: "Technology", count: 25 },
  { id: 2, name: "News", count: 18 },
  { id: 3, name: "Programming", count: 12 },
];

/**
 * Sets up all backend API mocks for a page
 * Call this in beforeEach hooks to mock all API endpoints
 */
export async function setupBackendAPIMocks(page: Page) {
  // Mock feed stats endpoint - matches both patterns used in app
  await page.route("**/v1/feeds/stats", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(MOCK_FEED_STATS),
    });
  });

  // Mock feeds fetch endpoint
  await page.route("**/v1/feeds/fetch/cursor**", async (route: Route) => {
    const response: MockFeedResponse = {
      data: MOCK_FEEDS,
      next_cursor: null,
    };

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });

  // Mock feeds list endpoint
  await page.route("**/v1/feeds", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ feeds: MOCK_FEEDS }),
    });
  });

  // Mock unread count endpoint
  await page.route("**/v1/feeds/count/unreads**", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(MOCK_UNREAD_COUNT),
    });
  });

  // Mock feed tags endpoint
  await page.route("**/v1/feeds/tags**", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ tags: MOCK_FEED_TAGS }),
    });
  });

  // Mock SSE endpoint for real-time updates
  await page.route("**/api/sse/updates**", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: `data: {"status": "connected"}\n\n`,
    });
  });

  // Mock auth session validation
  await page.route("**/sessions/whoami", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        id: "mock-session-id",
        active: true,
        identity: {
          id: "mock-user-id",
          schema_id: "default",
          traits: {
            email: "test@example.com",
            name: "Test User",
          },
        },
      }),
    });
  });
}

/**
 * Mock API error responses for error testing
 */
export async function setupAPIErrorMocks(page: Page) {
  // Mock 500 error for feed stats
  await page.route("**/v1/feeds/stats", async (route: Route) => {
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ error: "Internal server error" }),
    });
  });

  // Mock 404 error for feeds
  await page.route("**/v1/feeds/fetch/cursor**", async (route: Route) => {
    await route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: "Feeds not found" }),
    });
  });
}

/**
 * Mock slow API responses for timeout testing
 */
export async function setupSlowAPIMocks(page: Page, delay: number = 5000) {
  await page.route("**/v1/feeds/stats", async (route: Route) => {
    // Wait for specified delay
    await new Promise((resolve) => setTimeout(resolve, delay));

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(MOCK_FEED_STATS),
    });
  });
}

/**
 * Helper to create custom mock responses for specific endpoints
 */
export async function mockEndpoint(
  page: Page,
  pattern: string,
  response: any,
  status: number = 200,
) {
  await page.route(pattern, async (route: Route) => {
    await route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });
}
