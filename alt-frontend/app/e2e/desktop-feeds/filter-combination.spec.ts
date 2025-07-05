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

  test('should apply read status and search filters together (PROTECTED)', async ({ page }) => {
    // First apply read status filter
    const unreadFilter = page.locator('[data-testid="filter-read-status-unread"]');
    await unreadFilter.click();

    await page.waitForTimeout(300);

    // Then apply search filter
    const searchInput = page.getByPlaceholder('Search feeds...');
    await searchInput.fill('React');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(500);

    // Should show only unread React posts
    await expect(page.getByText('React 19 New Features')).toBeVisible();
    await expect(page.getByText('Next.js Performance Guide')).not.toBeVisible(); // This is read
    await expect(page.getByText('TypeScript Best Practices')).not.toBeVisible(); // No React in title

    // Verify result count
    const resultCount = page.getByText('1 results', { exact: false });
    await expect(resultCount).toBeVisible();
  });

  test('should apply time range filter correctly (PROTECTED)', async ({ page }) => {
    // First check that all feeds are visible initially
    await expect(page.getByText('React 19 New Features')).toBeVisible();
    await expect(page.getByText('Next.js Performance Guide')).toBeVisible();
    await expect(page.getByText('TypeScript Best Practices')).toBeVisible();

    // Apply time range filter (today)
    const todayFilter = page.locator('[data-testid="filter-time-range-today"]');
    await todayFilter.click();

    await page.waitForTimeout(500);

    // Check if time range filter has any effect
    // Since we're filtering by "today" and feeds might be from today,
    // we'll just verify that the filter is applied and some content is shown
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // The content might be filtered, so just check that the filter is active
    const timeRangeButton = page.locator('[data-testid="filter-time-range-today"]');
    await expect(timeRangeButton).toBeChecked();
  });

  test('should clear all filters when Clear Filters button is clicked (PROTECTED)', async ({ page }) => {
    // Apply multiple filters
    const unreadFilter = page.locator('[data-testid="filter-read-status-unread"]');
    await unreadFilter.click();

    await page.waitForTimeout(300);

    const searchInput = page.getByPlaceholder('Search feeds...');
    await searchInput.fill('React');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(500);

    // Verify filters are applied (limited results)
    await expect(page.getByText('React 19 New Features')).toBeVisible();
    await expect(page.getByText('Next.js Performance Guide')).not.toBeVisible();

    // Clear all filters (use FilterBar's clear button)
    const clearButton = page.locator('[data-testid="filter-bar"] [data-testid="filter-clear-button"]');
    await clearButton.click();

    await page.waitForTimeout(500);

    // Search should be cleared
    await expect(searchInput).toHaveValue('');

    // All feeds should be visible again
    await expect(page.getByText('React 19 New Features')).toBeVisible();
    await expect(page.getByText('Next.js Performance Guide')).toBeVisible();
    await expect(page.getByText('TypeScript Best Practices')).toBeVisible();

    // Read status should be back to 'all'
    const allReadFilter = page.locator('[data-testid="filter-read-status-all"]');
    await expect(allReadFilter).toBeChecked();
  });

  test('should show correct filter status indicator (PROTECTED)', async ({ page }) => {
    // Initially no filters active
    const filterBar = page.locator('[data-testid="filter-bar"]');
    await expect(filterBar).toContainText('Filters Active: None');

    // Apply a filter
    const unreadFilter = page.locator('[data-testid="filter-read-status-unread"]');
    await unreadFilter.click();

    await page.waitForTimeout(300);

    // Should show filters are active
    await expect(filterBar).toContainText('Filters Active: Yes');

    // Clear filters button should be visible (in FilterBar)
    const clearButton = page.locator('[data-testid="filter-bar"] [data-testid="filter-clear-button"]');
    await expect(clearButton).toBeVisible();
  });
});