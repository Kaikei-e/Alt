import { test, expect } from '@playwright/test';

test.describe('Filter Performance Optimization - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Mock large dataset for performance testing
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = Array.from({ length: 100 }, (_, i) => ({
        title: `Feed Article ${i + 1} ${i % 3 === 0 ? 'React' : i % 3 === 1 ? 'TypeScript' : 'JavaScript'}`,
        description: `This is a description for article ${i + 1} about web development and programming`,
        link: `https://example.com/article-${i + 1}`,
        published: new Date(Date.now() - (i * 60 * 60 * 1000)).toISOString(), // Spread across hours
      }));

      await route.fulfill({
        json: {
          data: feeds,
          next_cursor: null
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(2000); // Allow time for data loading
  });

  test('should filter large dataset efficiently (PROTECTED)', async ({ page }) => {
    // Verify all feeds are loaded
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // Check that many feeds are visible initially
    const feedCards = timeline.locator('div').filter({ hasText: 'Feed Article' });
    const initialCount = await feedCards.count();
    expect(initialCount).toBeGreaterThan(50); // Should have loaded most feeds

    // Apply search filter and measure performance
    const searchInput = page.getByPlaceholder('Search feeds...');
    const startTime = Date.now();

    await searchInput.fill('React');
    await page.keyboard.press('Enter');

    // Wait for filtering to complete
    await page.waitForTimeout(500);

    const endTime = Date.now();
    const filterTime = endTime - startTime;

    // Performance check: filtering should be fast (under 1.5 seconds, accounting for parallel test execution)
    expect(filterTime).toBeLessThan(1500);

    // Verify filtering worked
    const searchResults = page.getByText('results', { exact: false });
    await expect(searchResults).toBeVisible();

    // Should show only React articles
    const reactArticles = timeline.locator('div').filter({ hasText: 'React' });
    const reactCount = await reactArticles.count();
    expect(reactCount).toBeGreaterThan(0);
    expect(reactCount).toBeLessThan(initialCount);
  });

  test('should handle rapid filter changes without lag (PROTECTED)', async ({ page }) => {
    const searchInput = page.getByPlaceholder('Search feeds...');

    // Rapid search changes
    const searches = ['React', 'TypeScript', 'JavaScript', ''];

    for (const searchTerm of searches) {
      await searchInput.fill(searchTerm);
      await page.keyboard.press('Enter');
      await page.waitForTimeout(100); // Brief wait between searches

      // Verify the timeline is still responsive
      const timeline = page.locator('[data-testid="desktop-timeline"]');
      await expect(timeline).toBeVisible();
    }

    // Final state should show all feeds
    const allFeeds = page.locator('div').filter({ hasText: 'Feed Article' });
    const finalCount = await allFeeds.count();
    expect(finalCount).toBeGreaterThan(50);
  });

  test('should combine filters efficiently (PROTECTED)', async ({ page }) => {
    // Apply multiple filters in sequence
    const unreadFilter = page.locator('[data-testid="filter-read-status-unread"]');
    await unreadFilter.click();
    await page.waitForTimeout(200);

    const searchInput = page.getByPlaceholder('Search feeds...');
    await searchInput.fill('TypeScript');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(200);

    const todayFilter = page.locator('[data-testid="filter-time-range-today"]');
    await todayFilter.click();
    await page.waitForTimeout(200);

    // Verify all filters are active
    await expect(unreadFilter).toBeChecked();
    await expect(todayFilter).toBeChecked();

    // Timeline should still be responsive
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // Should have some filtering effect - check for timeline content
    const typeScriptArticles = timeline.locator('div').filter({ hasText: 'TypeScript' });
    const typeScriptCount = await typeScriptArticles.count();
    expect(typeScriptCount).toBeGreaterThan(0);
  });

  test('should maintain performance with scroll (PROTECTED)', async ({ page }) => {
    const timeline = page.locator('[data-testid="desktop-timeline"]');

    // Check initial state
    await expect(timeline).toBeVisible();

    // Scroll down to test virtual scrolling/pagination
    await timeline.evaluate(el => el.scrollTo(0, el.scrollHeight / 2));
    await page.waitForTimeout(300);

    // Apply filter while scrolled
    const searchInput = page.getByPlaceholder('Search feeds...');
    await searchInput.fill('JavaScript');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(500);

    // Should still be responsive
    await expect(timeline).toBeVisible();
    const jsArticles = timeline.locator('div').filter({ hasText: 'JavaScript' });
    const jsCount = await jsArticles.count();
    expect(jsCount).toBeGreaterThan(0);
  });
});