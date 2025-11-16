import type { Page } from "@playwright/test";
import {
  type ArticleData,
  createMockArticle,
  createMockArticlesResponse,
  createMockFeed,
  createMockFeedsResponse,
  type FeedData,
} from "./test-data";

/**
 * API mock helper functions for E2E tests
 */

/**
 * Mock feeds list API endpoint
 */
export async function mockFeedsApi(
  page: Page,
  feeds: FeedData[] | number = 10,
  hasMore = false,
) {
  const response =
    typeof feeds === "number"
      ? createMockFeedsResponse(feeds, hasMore)
      : { feeds, cursor: hasMore ? "next" : null, hasMore };

  await page.route("**/v1/feeds**", (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });
}

/**
 * Mock articles list API endpoint
 */
export async function mockArticlesApi(
  page: Page,
  articles: ArticleData[] | number = 20,
  hasMore = false,
) {
  const response =
    typeof articles === "number"
      ? createMockArticlesResponse(articles, hasMore)
      : { articles, cursor: hasMore ? "next" : null, hasMore };

  await page.route("**/v1/articles**", (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });
}

/**
 * Mock single feed API endpoint
 */
export async function mockFeedApi(page: Page, feed: Partial<FeedData>) {
  const mockFeed = createMockFeed(feed);

  await page.route("**/v1/feeds/*", (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(mockFeed),
    });
  });
}

/**
 * Mock single article API endpoint
 */
export async function mockArticleApi(
  page: Page,
  article: Partial<ArticleData>,
) {
  const mockArticle = createMockArticle(article);

  await page.route("**/v1/articles/*", (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(mockArticle),
    });
  });
}

/**
 * Mock empty feeds response
 */
export async function mockEmptyFeeds(page: Page) {
  await page.route("**/v1/feeds**", (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ feeds: [], cursor: null, hasMore: false }),
    });
  });
}

/**
 * Mock empty articles response
 */
export async function mockEmptyArticles(page: Page) {
  await page.route("**/v1/articles**", (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ articles: [], cursor: null, hasMore: false }),
    });
  });
}

/**
 * Mock API error response
 */
export async function mockApiError(
  page: Page,
  endpoint: string,
  statusCode = 500,
  errorMessage = "Internal Server Error",
) {
  await page.route(endpoint, (route) => {
    route.fulfill({
      status: statusCode,
      contentType: "application/json",
      body: JSON.stringify({ error: errorMessage }),
    });
  });
}

/**
 * Mock network timeout
 */
export async function mockNetworkTimeout(page: Page, endpoint: string) {
  await page.route(endpoint, (route) => {
    route.abort("timedout");
  });
}

/**
 * Mock network failure
 */
export async function mockNetworkFailure(page: Page, endpoint: string) {
  await page.route(endpoint, (route) => {
    route.abort("failed");
  });
}

/**
 * Mock successful feed creation
 */
export async function mockCreateFeedSuccess(
  page: Page,
  feed?: Partial<FeedData>,
) {
  const mockFeed = createMockFeed(feed);

  await page.route("**/v1/feeds", (route) => {
    if (route.request().method() === "POST") {
      route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify(mockFeed),
      });
    } else {
      route.continue();
    }
  });
}

/**
 * Mock feed creation error
 */
export async function mockCreateFeedError(
  page: Page,
  errorMessage = "Invalid feed URL",
) {
  await page.route("**/v1/feeds", (route) => {
    if (route.request().method() === "POST") {
      route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ error: errorMessage }),
      });
    } else {
      route.continue();
    }
  });
}

/**
 * Mock feed update success
 */
export async function mockUpdateFeedSuccess(page: Page, feedId: string) {
  await page.route(`**/v1/feeds/${feedId}`, (route) => {
    if (
      route.request().method() === "PUT" ||
      route.request().method() === "PATCH"
    ) {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true }),
      });
    } else {
      route.continue();
    }
  });
}

/**
 * Mock feed deletion success
 */
export async function mockDeleteFeedSuccess(page: Page, feedId: string) {
  await page.route(`**/v1/feeds/${feedId}`, (route) => {
    if (route.request().method() === "DELETE") {
      route.fulfill({
        status: 204,
      });
    } else {
      route.continue();
    }
  });
}

/**
 * Mock article mark as read
 */
export async function mockMarkAsRead(page: Page, articleId: string) {
  await page.route(`**/v1/articles/${articleId}/read`, (route) => {
    if (route.request().method() === "POST") {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true }),
      });
    } else {
      route.continue();
    }
  });
}

/**
 * Mock article mark as favorite
 */
export async function mockMarkAsFavorite(page: Page, articleId: string) {
  await page.route(`**/v1/articles/${articleId}/favorite`, (route) => {
    if (route.request().method() === "POST") {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true }),
      });
    } else {
      route.continue();
    }
  });
}

/**
 * Clear all API mocks
 */
export async function clearApiMocks(page: Page) {
  await page.unroute("**/*");
}

/**
 * Setup default API mocks for standard testing
 */
export async function setupDefaultMocks(page: Page) {
  await mockFeedsApi(page, 10);
  await mockArticlesApi(page, 20);
}
