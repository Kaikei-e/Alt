import { test, expect } from '@playwright/test';

test.describe('Filter Combination Logic - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Mock feed data with diverse metadata for filter testing
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = [
        {
          title: 'React 19 New Features',
          description: 'React team announces new features and TypeScript improvements',
          link: 'https://example.com/react-19',
          published: new Date().toISOString(),
          isRead: false,
          metadata: {
            source: { id: 'techcrunch', name: 'TechCrunch' },
            tags: ['react', 'javascript'],
            priority: 'high'
          }
        },
        {
          title: 'Next.js Performance Guide',
          description: 'Guide to optimizing Next.js applications',
          link: 'https://example.com/nextjs-perf',
          published: new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString(), // 2 days ago
          isRead: true,
          metadata: {
            source: { id: 'devto', name: 'Dev.to' },
            tags: ['nextjs', 'performance'],
            priority: 'medium'
          }
        },
        {
          title: 'TypeScript Best Practices',
          description: 'Modern TypeScript development practices',
          link: 'https://example.com/typescript-best',
          published: new Date(Date.now() - 10 * 24 * 60 * 60 * 1000).toISOString(), // 10 days ago
          isRead: false,
          metadata: {
            source: { id: 'medium', name: 'Medium' },
            tags: ['typescript', 'best-practices'],
            priority: 'low'
          }
        }
      ];

      await route.fulfill({
        json: {
          data: feeds,
          next_cursor: null
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);
  });
});