/**
 * Test data generators and fixtures for E2E tests
 */

/**
 * Test user credentials
 */
export const testUsers = {
  validUser: {
    email: "test@example.com",
    password: "password123",
    name: "Test User",
  },
  invalidUser: {
    email: "invalid@example.com",
    password: "wrongpassword",
  },
  newUser: {
    email: `test-${Date.now()}@example.com`,
    password: "newpassword123",
    name: "New Test User",
  },
} as const;

/**
 * Test feed data
 */
export const testFeeds = {
  techFeed: {
    url: "https://example.com/tech.rss",
    title: "Technology News",
    description: "Latest technology news and updates",
    category: "technology",
  },
  newsFeed: {
    url: "https://example.com/news.rss",
    title: "World News",
    description: "Global news coverage",
    category: "news",
  },
  blogFeed: {
    url: "https://example.com/blog.rss",
    title: "Tech Blog",
    description: "Personal tech blog",
    category: "blog",
  },
} as const;

/**
 * Test article data
 */
export const testArticles = {
  article1: {
    title: "Introduction to Next.js 15",
    content: "Next.js 15 brings amazing new features...",
    url: "https://example.com/articles/nextjs-15",
    publishedAt: "2025-01-01T00:00:00Z",
  },
  article2: {
    title: "React 19 Released",
    content: "React 19 is now available with...",
    url: "https://example.com/articles/react-19",
    publishedAt: "2025-01-02T00:00:00Z",
  },
  article3: {
    title: "TypeScript Best Practices",
    content: "Learn the best practices for TypeScript...",
    url: "https://example.com/articles/typescript",
    publishedAt: "2025-01-03T00:00:00Z",
  },
} as const;

/**
 * Generate random email for testing
 */
export function generateRandomEmail(): string {
  const timestamp = Date.now();
  const random = Math.random().toString(36).substring(7);
  return `test-${timestamp}-${random}@example.com`;
}

/**
 * Generate random feed URL
 */
export function generateRandomFeedUrl(): string {
  const random = Math.random().toString(36).substring(7);
  return `https://example-${random}.com/feed.rss`;
}

/**
 * Create mock feed response
 */
export function createMockFeed(overrides?: Partial<FeedData>): FeedData {
  return {
    id: `feed-${Date.now()}`,
    title: "Test Feed",
    description: "A test feed for E2E testing",
    url: "https://example.com/feed.rss",
    lastUpdated: new Date().toISOString(),
    unreadCount: 5,
    totalCount: 100,
    ...overrides,
  };
}

/**
 * Create mock article response
 */
export function createMockArticle(overrides?: Partial<ArticleData>): ArticleData {
  return {
    id: `article-${Date.now()}`,
    title: "Test Article",
    content: "This is a test article content",
    url: "https://example.com/article",
    publishedAt: new Date().toISOString(),
    isRead: false,
    isFavorite: false,
    feedId: "feed-123",
    feedTitle: "Test Feed",
    ...overrides,
  };
}

/**
 * Create multiple mock feeds
 */
export function createMockFeeds(count: number): FeedData[] {
  return Array.from({ length: count }, (_, i) =>
    createMockFeed({
      id: `feed-${i}`,
      title: `Test Feed ${i + 1}`,
      unreadCount: Math.floor(Math.random() * 20),
    })
  );
}

/**
 * Create multiple mock articles
 */
export function createMockArticles(count: number, feedId?: string): ArticleData[] {
  return Array.from({ length: count }, (_, i) =>
    createMockArticle({
      id: `article-${i}`,
      title: `Test Article ${i + 1}`,
      feedId: feedId || `feed-${Math.floor(Math.random() * 5)}`,
    })
  );
}

/**
 * Type definitions for test data
 */
export interface FeedData {
  id: string;
  title: string;
  description: string;
  url: string;
  lastUpdated: string;
  unreadCount: number;
  totalCount: number;
}

export interface ArticleData {
  id: string;
  title: string;
  content: string;
  url: string;
  publishedAt: string;
  isRead: boolean;
  isFavorite: boolean;
  feedId: string;
  feedTitle: string;
}

/**
 * API response types
 */
export interface FeedsResponse {
  feeds: FeedData[];
  cursor: string | null;
  hasMore: boolean;
}

export interface ArticlesResponse {
  articles: ArticleData[];
  cursor: string | null;
  hasMore: boolean;
}

/**
 * Create mock feeds API response
 */
export function createMockFeedsResponse(count: number, hasMore = false): FeedsResponse {
  return {
    feeds: createMockFeeds(count),
    cursor: hasMore ? "next-cursor" : null,
    hasMore,
  };
}

/**
 * Create mock articles API response
 */
export function createMockArticlesResponse(count: number, hasMore = false): ArticlesResponse {
  return {
    articles: createMockArticles(count),
    cursor: hasMore ? "next-cursor" : null,
    hasMore,
  };
}
