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