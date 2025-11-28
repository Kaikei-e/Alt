import { type Page, type Route } from '@playwright/test';
import feedsData from '../fixtures/feeds.json' with { type: 'json' };
import feedsEmpty from '../fixtures/feeds-empty.json' with { type: 'json' };
import feedsPage2 from '../fixtures/feeds-page2.json' with { type: 'json' };
import articleDetail from '../fixtures/article-detail.json' with { type: 'json' };
import errors from '../fixtures/errors.json' with { type: 'json' };

/**
 * Mock API responses for E2E tests
 * This utility provides helper functions to set up network interception
 */

/**
 * Mock feeds API endpoint
 * @param page - Playwright page instance
 * @param options - Configuration options
 */
export async function mockFeedsApi(
  page: Page,
  options: {
    empty?: boolean;
    hasMore?: boolean;
    delay?: number;
  } = {},
) {
  // Mock both /api/frontend/feeds/fetch/cursor (client-side) and /v1/feeds/fetch/cursor (server-side via API route)
  const handleRoute = async (route: Route) => {
    const url = new URL(route.request().url());
    const cursor = url.searchParams.get('cursor');

    // If cursor is provided (pagination), return page 2 data
    if (cursor && !options.empty) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(feedsPage2),
      });
      return;
    }

    // Return empty data if requested
    if (options.empty) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(feedsEmpty),
      });
      return;
    }

    // Return feeds with hasMore option
    const response = {
      ...feedsData,
      has_more: options.hasMore !== undefined ? options.hasMore : feedsData.has_more,
    };

    if (options.delay) {
      await new Promise((resolve) => setTimeout(resolve, options.delay));
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(response),
    });
  };

  // Mock client-side API route (with or without query parameters)
  await page.route('**/api/frontend/feeds/fetch/cursor*', handleRoute);
  // Mock Next.js API route that proxies to backend
  await page.route('**/api/frontend/v1/feeds/fetch/cursor*', handleRoute);
  // Mock direct backend API (used by server-side rendering via serverFetch)
  // Note: serverFetch uses process.env.API_URL directly, so we need to mock the full URL
  await page.route('**/v1/feeds/fetch/cursor*', handleRoute);
}

/**
 * Mock article detail API endpoint
 * @param page - Playwright page instance
 */
export async function mockArticleDetailApi(page: Page) {
  // Match both /api/frontend/v1/articles/:id and /v1/articles/:id patterns
  await page.route('**/api/frontend/v1/articles/**', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(articleDetail),
    });
  });
  await page.route('**/v1/articles/**', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(articleDetail),
    });
  });
}

/**
 * Mock article content API endpoint
 * @param page - Playwright page instance
 */
export async function mockArticleContentApi(page: Page) {
  await page.route('**/api/frontend/articles/content**', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(articleDetail),
    });
  });
}

/**
 * Mock article read status API endpoint
 * @param page - Playwright page instance
 */
export async function mockArticleReadApi(page: Page) {
  await page.route('**/api/frontend/feeds/read**', async (route: Route) => {
    const request = route.request();
    const method = request.method();

    if (method === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ message: 'Marked as read' }),
      });
    } else {
      await route.fulfill({
        status: 405,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Method not allowed' }),
      });
    }
  });
}

/**
 * Mock bookmark API endpoint
 * @param page - Playwright page instance
 */
export async function mockBookmarkApi(page: Page) {
  await page.route('**/api/frontend/bookmarks**', async (route: Route) => {
    const request = route.request();
    const method = request.method();

    if (method === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ message: 'Bookmarked successfully' }),
      });
    } else if (method === 'DELETE') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ message: 'Bookmark removed' }),
      });
    } else {
      await route.fulfill({
        status: 405,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Method not allowed' }),
      });
    }
  });
}

/**
 * Mock search API endpoint
 * @param page - Playwright page instance
 * @param options - Configuration options
 */
export async function mockSearchApi(
  page: Page,
  options: {
    empty?: boolean;
    query?: string;
  } = {},
) {
  // Mock both feed search and article search endpoints
  // Use * instead of ** to match URLs with query parameters
  await page.route('**/api/frontend/v1/feeds/search*', async (route: Route) => {
    if (options.empty) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ results: [] }),
      });
      return;
    }

    // Filter feeds based on query if provided
    const query = options.query || 'AI';
    const filteredFeeds = feedsData.data.filter(
      (feed) =>
        feed.title.toLowerCase().includes(query.toLowerCase()) ||
        feed.description.toLowerCase().includes(query.toLowerCase()),
    );

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ results: filteredFeeds }),
    });
  });

  // Mock article search endpoint (used by DesktopArticlesSearchPage and mobile search)
  // ArticleApi.searchArticles uses GET /v1/articles/search?q=...
  // Which becomes /api/frontend/v1/articles/search?q=... on client-side
  await page.route('**/api/frontend/v1/articles/search*', async (route: Route) => {
    if (options.empty) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
      return;
    }

    // Filter feeds based on query if provided and convert to Article format
    const query = options.query || 'AI';
    const filteredFeeds = feedsData.data.filter(
      (feed) =>
        feed.title.toLowerCase().includes(query.toLowerCase()) ||
        feed.description.toLowerCase().includes(query.toLowerCase()),
    );

    // Convert to Article format
    const articles = filteredFeeds.map((feed) => ({
      id: feed.id,
      title: feed.title,
      content: feed.description,
      url: feed.link,
      published_at: feed.published,
      tags: feed.tags || [],
    }));

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(articles),
    });
  });

  // Also mock direct backend endpoint (for server-side rendering)
  await page.route('**/v1/articles/search*', async (route: Route) => {
    if (options.empty) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
      return;
    }

    // Filter feeds based on query if provided and convert to Article format
    const query = options.query || 'AI';
    const filteredFeeds = feedsData.data.filter(
      (feed) =>
        feed.title.toLowerCase().includes(query.toLowerCase()) ||
        feed.description.toLowerCase().includes(query.toLowerCase()),
    );

    // Convert to Article format
    const articles = filteredFeeds.map((feed) => ({
      id: feed.id,
      title: feed.title,
      content: feed.description,
      url: feed.link,
      published_at: feed.published,
      tags: feed.tags || [],
    }));

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(articles),
    });
  });
}

/**
 * Mock error response
 * @param page - Playwright page instance
 * @param endpoint - API endpoint pattern to match
 * @param statusCode - HTTP status code (404, 401, 500)
 */
export async function mockErrorResponse(
  page: Page,
  endpoint: string,
  statusCode: 404 | 401 | 500 = 500,
) {
  await page.route(`**${endpoint}**`, async (route: Route) => {
    const statusKey = statusCode.toString() as keyof typeof errors;
    const errorResponse = errors[statusKey];
    await route.fulfill({
      status: statusCode,
      contentType: 'application/json',
      body: JSON.stringify(errorResponse),
    });
  });
}

/**
 * Setup all common API mocks for feed-related tests
 * @param page - Playwright page instance
 */
export async function setupFeedMocks(page: Page) {
  await mockFeedsApi(page);
  await mockArticleContentApi(page);
  await mockArticleDetailApi(page);
  await mockArticleReadApi(page);
  await mockBookmarkApi(page);
}

