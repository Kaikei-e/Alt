import { test, expect } from '@playwright/test';

test.describe('URL Filter Persistence - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Mock feed data
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = [
        {
          title: 'React 19 Features',
          description: 'New React features announcement',
          link: 'https://example.com/react-19',
          published: new Date().toISOString(),
        },
        {
          title: 'TypeScript Updates',
          description: 'Latest TypeScript improvements',
          link: 'https://example.com/typescript',
          published: new Date().toISOString(),
        }
      ];

      await route.fulfill({
        json: { data: feeds, next_cursor: null }
      });
    });
  });

  test('should persist search query in URL (PROTECTED)', async ({ page }) => {
    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);

    // Apply search
    const searchInput = page.getByPlaceholder('Search feeds...');
    await searchInput.fill('React');
    await page.keyboard.press('Enter');

    // Wait for URL to update
    await page.waitForTimeout(1000);

    // Check URL contains search parameter
    const url = page.url();
    expect(url).toContain('search=React');

    // Refresh page and verify search is restored
    await page.reload();
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);

    // Search input should have the value from URL
    await expect(searchInput).toHaveValue('React');

    // Search results should be displayed
    const searchHeader = page.getByText('Search:', { exact: false });
    await expect(searchHeader).toBeVisible();
  });

  test('should persist filter state in URL (PROTECTED)', async ({ page }) => {
    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);

    // Apply read status filter
    const unreadFilter = page.locator('[data-testid="filter-read-status-unread"]');
    await unreadFilter.click();

    // Apply time range filter
    const todayFilter = page.locator('[data-testid="filter-time-range-today"]');
    await todayFilter.click();

    // Wait for URL to update
    await page.waitForTimeout(1000);

    // Check URL contains filter parameters
    const url = page.url();
    expect(url).toContain('readStatus=unread');
    expect(url).toContain('timeRange=today');

    // Refresh page and verify filters are restored
    await page.reload();
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);

    // Filters should be applied from URL
    await expect(unreadFilter).toBeChecked();
    await expect(todayFilter).toBeChecked();
  });

  test('should clear URL parameters when clearing filters (PROTECTED)', async ({ page }) => {
    // Start with URL that has filter parameters
    await page.goto('/desktop/feeds?search=React&readStatus=unread&timeRange=today');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);

    // Verify filters are applied from URL
    const searchInput = page.getByPlaceholder('Search feeds...');
    await expect(searchInput).toHaveValue('React');

    const unreadFilter = page.locator('[data-testid="filter-read-status-unread"]');
    await expect(unreadFilter).toBeChecked();

    // Clear all filters (use FilterBar's clear button)
    const clearButton = page.locator('[data-testid="filter-bar"] [data-testid="filter-clear-button"]');
    await clearButton.click();

    // Wait for URL to update
    await page.waitForTimeout(1000);

    // URL should be clean
    const url = page.url();
    expect(url).toBe(page.url().split('?')[0]); // No query parameters

    // Search input should be empty
    await expect(searchInput).toHaveValue('');

    // Filters should be reset
    const allReadFilter = page.locator('[data-testid="filter-read-status-all"]');
    await expect(allReadFilter).toBeChecked();
  });

  test('should handle invalid URL parameters gracefully (PROTECTED)', async ({ page }) => {
    // Navigate with invalid filter values
    await page.goto('/desktop/feeds?readStatus=invalid&timeRange=badvalue&search=test');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(2000); // Increased wait time

    // Should default to valid values and not crash
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // Should show search even with invalid filters
    const searchInput = page.getByPlaceholder('Search feeds...');
    await expect(searchInput).toHaveValue('test');

    // Wait for URL filters hook to process and update UI
    await page.waitForTimeout(1000);

    // Invalid filter values should default to 'all'
    const allReadFilter = page.locator('[data-testid="filter-read-status-all"]');
    await expect(allReadFilter).toBeChecked();
  });
});