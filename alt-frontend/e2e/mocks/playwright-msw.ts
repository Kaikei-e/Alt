import type { Page, Route } from "@playwright/test";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));

interface FeedItem {
  id: string;
  title: string;
  description: string;
  link: string;
  published: string;
  tags?: string[];
}

// Load JSON fixtures
const feedsData = JSON.parse(
  readFileSync(join(__dirname, "../fixtures/feeds.json"), "utf-8"),
);
const feedsEmpty = JSON.parse(
  readFileSync(join(__dirname, "../fixtures/feeds-empty.json"), "utf-8"),
);
const feedsPage2 = JSON.parse(
  readFileSync(join(__dirname, "../fixtures/feeds-page2.json"), "utf-8"),
);
const articleDetail = JSON.parse(
  readFileSync(join(__dirname, "../fixtures/article-detail.json"), "utf-8"),
);
const errors = JSON.parse(
  readFileSync(join(__dirname, "../fixtures/errors.json"), "utf-8"),
);

/**
 * Options for mocking API responses
 */
export interface MockOptions {
  empty?: boolean;
  hasMore?: boolean;
  delay?: number;
  errorStatus?: 401 | 404 | 500;
}

/**
 * Setup all API mocks for Playwright tests
 * This handles client-side requests via page.route()
 */
export async function setupPlaywrightMocks(
  page: Page,
  options: MockOptions = {},
): Promise<void> {
  await setupFeedMocks(page, options);
  await setupArticleMocks(page, options);
  await setupSearchMocks(page, options);
  await setupBookmarkMocks(page);
  await setupReadStatusMocks(page);
}

/**
 * Setup feed-related API mocks
 */
async function setupFeedMocks(page: Page, options: MockOptions): Promise<void> {
  const feedPatterns = [
    "**/api/frontend/feeds/fetch/cursor*",
    "**/api/frontend/v1/feeds/fetch/cursor*",
    "**/v1/feeds/fetch/cursor*",
  ];

  for (const pattern of feedPatterns) {
    await page.route(pattern, async (route: Route) => {
      // Check URL params for mock scenarios
      const url = new URL(route.request().url());
      const mockEmpty = url.searchParams.get("mock_empty");
      const mockError = url.searchParams.get("mock_error");
      const cursor = url.searchParams.get("cursor");

      // Handle error scenarios from URL param
      if (mockError || options.errorStatus) {
        const statusCode = mockError
          ? parseInt(mockError, 10)
          : options.errorStatus;
        const statusKey = statusCode?.toString() as keyof typeof errors;
        await route.fulfill({
          status: statusCode,
          contentType: "application/json",
          body: JSON.stringify(errors[statusKey] || errors["500"]),
        });
        return;
      }

      // Handle empty scenario
      if (mockEmpty === "true" || options.empty) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(feedsEmpty),
        });
        return;
      }

      // Handle pagination
      if (cursor) {
        if (options.delay) {
          await new Promise((resolve) => setTimeout(resolve, options.delay));
        }
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(feedsPage2),
        });
        return;
      }

      // Default response
      const response = {
        ...feedsData,
        has_more:
          options.hasMore !== undefined ? options.hasMore : feedsData.has_more,
      };

      if (options.delay) {
        await new Promise((resolve) => setTimeout(resolve, options.delay));
      }

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(response),
      });
    });
  }

  // Feed stats
  await page.route("**/v1/feeds/stats*", async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        totalFeeds: 42,
        totalArticles: 1337,
        unreadCount: 15,
        dailyReadCount: 23,
      }),
    });
  });
}

/**
 * Setup article-related API mocks
 */
async function setupArticleMocks(
  page: Page,
  options: MockOptions,
): Promise<void> {
  const articlePatterns = ["**/api/frontend/v1/articles/*", "**/v1/articles/*"];

  for (const pattern of articlePatterns) {
    await page.route(pattern, async (route: Route) => {
      const url = new URL(route.request().url());
      const mockError = url.searchParams.get("mock_error");

      // Skip search endpoint (handled separately)
      if (url.pathname.includes("/search")) {
        await route.continue();
        return;
      }

      // Handle error scenarios
      if (mockError || options.errorStatus) {
        const statusCode = mockError
          ? parseInt(mockError, 10)
          : options.errorStatus;
        const statusKey = statusCode?.toString() as keyof typeof errors;
        await route.fulfill({
          status: statusCode,
          contentType: "application/json",
          body: JSON.stringify(errors[statusKey] || errors["500"]),
        });
        return;
      }

      // Extract article ID from URL
      const pathParts = url.pathname.split("/");
      const articleId = pathParts[pathParts.length - 1];

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          ...articleDetail,
          id: articleId,
        }),
      });
    });
  }

  // Article content endpoint
  await page.route(
    "**/api/frontend/articles/content*",
    async (route: Route) => {
      if (options.errorStatus) {
        const statusKey = options.errorStatus.toString() as keyof typeof errors;
        await route.fulfill({
          status: options.errorStatus,
          contentType: "application/json",
          body: JSON.stringify(errors[statusKey]),
        });
        return;
      }

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(articleDetail),
      });
    },
  );
}

/**
 * Setup search API mocks
 */
async function setupSearchMocks(
  page: Page,
  options: MockOptions,
): Promise<void> {
  const searchPatterns = [
    "**/api/frontend/v1/articles/search*",
    "**/api/frontend/v1/feeds/search*",
    "**/v1/articles/search*",
  ];

  for (const pattern of searchPatterns) {
    await page.route(pattern, async (route: Route) => {
      const url = new URL(route.request().url());
      const query = url.searchParams.get("q") || "";
      const mockEmpty = url.searchParams.get("mock_empty");

      // Handle empty scenarios
      if (
        mockEmpty === "true" ||
        options.empty ||
        query.includes("NonExistent")
      ) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([]),
        });
        return;
      }

      // Filter feeds based on query
      const filteredFeeds = feedsData.data.filter(
        (feed: FeedItem) =>
          feed.title.toLowerCase().includes(query.toLowerCase()) ||
          feed.description.toLowerCase().includes(query.toLowerCase()),
      );

      // Convert to Article format
      const articles = filteredFeeds.map((feed: FeedItem) => ({
        id: feed.id,
        title: feed.title,
        content: feed.description,
        url: feed.link,
        published_at: feed.published,
        tags: feed.tags || [],
      }));

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(articles),
      });
    });
  }
}

/**
 * Setup bookmark API mocks
 */
async function setupBookmarkMocks(page: Page): Promise<void> {
  await page.route("**/api/frontend/bookmarks*", async (route: Route) => {
    const method = route.request().method();

    if (method === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Bookmarked successfully" }),
      });
    } else if (method === "DELETE") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Bookmark removed" }),
      });
    } else {
      await route.fulfill({
        status: 405,
        contentType: "application/json",
        body: JSON.stringify({ error: "Method not allowed" }),
      });
    }
  });
}

/**
 * Setup read status API mocks
 */
async function setupReadStatusMocks(page: Page): Promise<void> {
  await page.route("**/api/frontend/feeds/read*", async (route: Route) => {
    const method = route.request().method();

    if (method === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Marked as read" }),
      });
    } else {
      await route.fulfill({
        status: 405,
        contentType: "application/json",
        body: JSON.stringify({ error: "Method not allowed" }),
      });
    }
  });
}

/**
 * Mock specific endpoint with error response
 */
export async function mockErrorResponse(
  page: Page,
  endpoint: string,
  statusCode: 404 | 401 | 500 = 500,
): Promise<void> {
  await page.route(`**${endpoint}**`, async (route: Route) => {
    const statusKey = statusCode.toString() as keyof typeof errors;
    await route.fulfill({
      status: statusCode,
      contentType: "application/json",
      body: JSON.stringify(errors[statusKey]),
    });
  });
}

/**
 * Mock feeds API with specific options
 */
export async function mockFeedsApi(
  page: Page,
  options: MockOptions = {},
): Promise<void> {
  await setupFeedMocks(page, options);
}

/**
 * Mock article detail API with specific options
 */
export async function mockArticleDetailApi(
  page: Page,
  options: MockOptions = {},
): Promise<void> {
  await setupArticleMocks(page, options);
}

/**
 * Mock search API with specific options
 */
export async function mockSearchApi(
  page: Page,
  options: MockOptions = {},
): Promise<void> {
  await setupSearchMocks(page, options);
}
