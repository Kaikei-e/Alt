import { http, HttpResponse } from 'msw';
import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

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
  readFileSync(join(__dirname, '../fixtures/feeds.json'), 'utf-8'),
);
const feedsEmpty = JSON.parse(
  readFileSync(join(__dirname, '../fixtures/feeds-empty.json'), 'utf-8'),
);
const feedsPage2 = JSON.parse(
  readFileSync(join(__dirname, '../fixtures/feeds-page2.json'), 'utf-8'),
);
const articleDetail = JSON.parse(
  readFileSync(join(__dirname, '../fixtures/article-detail.json'), 'utf-8'),
);
const errors = JSON.parse(
  readFileSync(join(__dirname, '../fixtures/errors.json'), 'utf-8'),
);

// Base URLs
const MOCK_API_BASE = 'http://localhost:4545';
const BACKEND_API_BASE = 'http://localhost:9000';

/**
 * Default MSW handlers for E2E tests
 * These mock both the mock-auth-service and real backend endpoints
 */
export const handlers = [
  // ============================================
  // Feed endpoints
  // ============================================

  // Cursor-based feed fetch
  http.get(`${MOCK_API_BASE}/v1/feeds/fetch/cursor`, ({ request }) => {
    const url = new URL(request.url);
    const cursor = url.searchParams.get('cursor');
    const mockEmpty = url.searchParams.get('empty');
    const mockError = url.searchParams.get('error');

    // Handle error scenarios
    if (mockError) {
      const statusCode = parseInt(mockError, 10) as 404 | 401 | 500;
      const statusKey = statusCode.toString() as keyof typeof errors;
      return HttpResponse.json(errors[statusKey], { status: statusCode });
    }

    // Handle empty scenario
    if (mockEmpty === 'true') {
      return HttpResponse.json(feedsEmpty);
    }

    // Handle pagination
    if (cursor) {
      return HttpResponse.json(feedsPage2);
    }

    return HttpResponse.json(feedsData);
  }),

  // Also handle backend URL pattern
  http.get(`${BACKEND_API_BASE}/v1/feeds/fetch/cursor`, ({ request }) => {
    const url = new URL(request.url);
    const cursor = url.searchParams.get('cursor');

    if (cursor) {
      return HttpResponse.json(feedsPage2);
    }
    return HttpResponse.json(feedsData);
  }),

  // Feed stats
  http.get(`${MOCK_API_BASE}/v1/feeds/stats`, () => {
    return HttpResponse.json({
      totalFeeds: 42,
      totalArticles: 1337,
      unreadCount: 15,
      dailyReadCount: 23,
    });
  }),

  http.get(`${BACKEND_API_BASE}/v1/feeds/stats`, () => {
    return HttpResponse.json({
      totalFeeds: 42,
      totalArticles: 1337,
      unreadCount: 15,
      dailyReadCount: 23,
    });
  }),

  // Mark as read
  http.post(`${MOCK_API_BASE}/v1/feeds/read`, () => {
    return HttpResponse.json({ message: 'Marked as read' });
  }),

  http.post(`${BACKEND_API_BASE}/v1/feeds/read`, () => {
    return HttpResponse.json({ message: 'Marked as read' });
  }),

  // ============================================
  // Article endpoints
  // ============================================

  // Article detail
  http.get(`${MOCK_API_BASE}/v1/articles/:id`, ({ params, request }) => {
    const { id } = params;
    const url = new URL(request.url);
    const mockError = url.searchParams.get('error');

    if (mockError) {
      const statusCode = parseInt(mockError, 10) as 404 | 401 | 500;
      const statusKey = statusCode.toString() as keyof typeof errors;
      return HttpResponse.json(errors[statusKey], { status: statusCode });
    }

    return HttpResponse.json({
      ...articleDetail,
      id,
    });
  }),

  http.get(`${BACKEND_API_BASE}/v1/articles/:id`, ({ params }) => {
    const { id } = params;
    return HttpResponse.json({
      ...articleDetail,
      id,
    });
  }),

  // Article search
  http.get(`${MOCK_API_BASE}/v1/articles/search`, ({ request }) => {
    const url = new URL(request.url);
    const query = url.searchParams.get('q') || '';
    const mockEmpty = url.searchParams.get('empty');

    if (mockEmpty === 'true' || query.includes('NonExistent')) {
      return HttpResponse.json([]);
    }

    const filteredFeeds = feedsData.data.filter(
      (feed: FeedItem) =>
        feed.title.toLowerCase().includes(query.toLowerCase()) ||
        feed.description.toLowerCase().includes(query.toLowerCase()),
    );

    return HttpResponse.json(
      filteredFeeds.map((feed: FeedItem) => ({
        id: feed.id,
        title: feed.title,
        content: feed.description,
        url: feed.link,
        published_at: feed.published,
        tags: feed.tags || [],
      })),
    );
  }),

  http.get(`${BACKEND_API_BASE}/v1/articles/search`, ({ request }) => {
    const url = new URL(request.url);
    const query = url.searchParams.get('q') || '';

    const filteredFeeds = feedsData.data.filter(
      (feed: FeedItem) =>
        feed.title.toLowerCase().includes(query.toLowerCase()) ||
        feed.description.toLowerCase().includes(query.toLowerCase()),
    );

    return HttpResponse.json(
      filteredFeeds.map((feed: FeedItem) => ({
        id: feed.id,
        title: feed.title,
        content: feed.description,
        url: feed.link,
        published_at: feed.published,
        tags: feed.tags || [],
      })),
    );
  }),

  // ============================================
  // Bookmark endpoints
  // ============================================

  http.post(`${MOCK_API_BASE}/v1/bookmarks`, () => {
    return HttpResponse.json({ message: 'Bookmarked successfully' });
  }),

  http.delete(`${MOCK_API_BASE}/v1/bookmarks/:id`, () => {
    return HttpResponse.json({ message: 'Bookmark removed' });
  }),

  // ============================================
  // Auth endpoints
  // ============================================

  http.get(`${MOCK_API_BASE}/sessions/whoami`, () => {
    return HttpResponse.json({
      id: 'test-session-id',
      active: true,
      identity: {
        id: 'test-user-id',
        schema_id: 'default',
        traits: {
          email: 'test@example.com',
          name: 'Test User',
        },
      },
    });
  }),

  http.get(`${MOCK_API_BASE}/session`, () => {
    return HttpResponse.json({
      id: 'test-session-id',
      active: true,
      identity: {
        id: 'test-user-id',
        schema_id: 'default',
        traits: {
          email: 'test@example.com',
          name: 'Test User',
        },
      },
    });
  }),

  // Health check
  http.get(`${MOCK_API_BASE}/v1/health`, () => {
    return HttpResponse.json({ status: 'ok', service: 'mock-msw' });
  }),
];

/**
 * Create an error handler for a specific endpoint
 */
export function createErrorHandler(
  endpoint: string,
  method: 'get' | 'post' | 'put' | 'delete',
  statusCode: 404 | 401 | 500,
) {
  const statusKey = statusCode.toString() as keyof typeof errors;
  const httpMethod = http[method];

  return httpMethod(`${MOCK_API_BASE}${endpoint}`, () => {
    return HttpResponse.json(errors[statusKey], { status: statusCode });
  });
}

/**
 * Handler for empty feeds response
 */
export const emptyFeedsHandler = http.get(
  `${MOCK_API_BASE}/v1/feeds/fetch/cursor`,
  () => HttpResponse.json(feedsEmpty),
);

/**
 * Handler for empty search response
 */
export const emptySearchHandler = http.get(
  `${MOCK_API_BASE}/v1/articles/search`,
  () => HttpResponse.json([]),
);
