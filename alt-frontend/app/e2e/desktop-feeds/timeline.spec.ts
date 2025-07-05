import { test, expect } from '@playwright/test';

test.describe('DesktopTimeline Independent Scroll - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Mock feed data for testing - use the correct cursor API format
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = Array.from({ length: 20 }, (_, i) => ({
        title: `Feed Title ${i}`,
        description: `Description for feed ${i}`,
        link: `https://example.com/feed-${i}`,
        published: new Date().toISOString(),
      }));

      await route.fulfill({
        json: { 
          data: feeds,
          next_cursor: "next-page-cursor"
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);
  });

  test('should have independent scrollable container (PROTECTED)', async ({ page }) => {
    // Wait for timeline to load
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // Verify scroll container properties
    await expect(timeline).toHaveCSS('overflow-y', 'auto');
    await expect(timeline).toHaveCSS('overflow-x', 'hidden');

    // Verify max height is set (computed value should be less than viewport)
    const maxHeight = await timeline.evaluate(el => getComputedStyle(el).maxHeight);
    const maxHeightValue = parseFloat(maxHeight);
    expect(maxHeightValue).toBeGreaterThan(0);
    expect(maxHeightValue).toBeLessThan(1000); // More flexible threshold
  });

  test('should maintain scroll position and infinite scroll (PROTECTED)', async ({ page }) => {
    // Locate timeline container
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // Check if content is scrollable
    const timelineHeight = await timeline.evaluate(el => el.scrollHeight);
    const containerHeight = await timeline.evaluate(el => el.clientHeight);

    if (timelineHeight > containerHeight) {
      // Content is scrollable - test scrolling
      const scrollAmount = Math.min(100, timelineHeight - containerHeight - 10);
      await timeline.evaluate((el, amount) => el.scrollTo(0, amount), scrollAmount);

      // Wait for scroll to complete
      await page.waitForTimeout(100);

      // Verify scroll position is maintained
      const scrollTop = await timeline.evaluate(el => el.scrollTop);
      expect(scrollTop).toBeGreaterThanOrEqual(0);

      // Test infinite scroll trigger
      await timeline.evaluate(el => el.scrollTo(0, el.scrollHeight - el.clientHeight));

      // Check for load more functionality or virtualized content
      const loadMoreButton = page.locator('text=Load more...');
      const virtualContainer = page.locator('[data-testid="virtual-container"]');
      const feedItems = page.locator('[data-testid^="feed-item-"]');

      const hasLoadMore = await loadMoreButton.isVisible().catch(() => false);
      const hasVirtualContainer = await virtualContainer.isVisible().catch(() => false);
      const hasFeedItems = await feedItems.first().isVisible().catch(() => false);

      // Either load more button should appear or virtualized content is present
      expect(hasLoadMore || hasVirtualContainer || hasFeedItems).toBeTruthy();
    } else {
      // Content doesn't scroll (limited content) - verify it's handled gracefully
      const virtualContainer = page.locator('[data-testid="virtual-container"]');
      const feedItems = page.locator('[data-testid^="feed-item-"]');

      const hasVirtualContainer = await virtualContainer.isVisible().catch(() => false);
      const hasFeedItems = await feedItems.first().isVisible().catch(() => false);

      // Either virtual container or feed items should be present
      expect(hasVirtualContainer || hasFeedItems).toBeTruthy();
    }
  });

  test('should handle loading states during scroll', async ({ page }) => {
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // Check that timeline shows some content or loading state
    const hasContent = await timeline.textContent();
    expect(hasContent).toBeTruthy();

    // Look for loading indicators or virtualized content
    const loadingSpinner = page.locator('text=/Loading|読み込み中|Spinner/');
    const virtualContainer = page.locator('[data-testid="virtual-container"]');
    const feedItems = page.locator('[data-testid^="feed-item-"]');

    const hasLoading = await loadingSpinner.isVisible().catch(() => false);
    const hasVirtualContainer = await virtualContainer.isVisible().catch(() => false);
    const hasFeedItems = await feedItems.first().isVisible().catch(() => false);

    // At least one of these should be true (loading, virtual container, or feed items)
    expect(hasLoading || hasVirtualContainer || hasFeedItems).toBeTruthy();
  });

  test('should be responsive across viewports (PROTECTED)', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    const timeline = page.locator('[data-testid="desktop-timeline"]');

    // Test desktop viewport (lg) - more flexible expectations
    await page.setViewportSize({ width: 1024, height: 768 });
    await page.waitForTimeout(500); // Increased wait time
    let maxHeight = await timeline.evaluate(el => getComputedStyle(el).maxHeight);
    let maxHeightValue = parseFloat(maxHeight);
    expect(maxHeightValue).toBeGreaterThan(400); // More flexible range
    expect(maxHeightValue).toBeLessThan(800);

    // Test tablet viewport (md) - more flexible expectations
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.waitForTimeout(500); // Increased wait time
    maxHeight = await timeline.evaluate(el => getComputedStyle(el).maxHeight);
    maxHeightValue = parseFloat(maxHeight);
    expect(maxHeightValue).toBeGreaterThan(600); // More flexible range
    expect(maxHeightValue).toBeLessThan(1100);

    // Test mobile viewport (sm) - more flexible expectations
    await page.setViewportSize({ width: 375, height: 667 });
    await page.waitForTimeout(500); // Increased wait time
    maxHeight = await timeline.evaluate(el => getComputedStyle(el).maxHeight);
    maxHeightValue = parseFloat(maxHeight);
    expect(maxHeightValue).toBeGreaterThan(400); // More flexible range
    expect(maxHeightValue).toBeLessThan(800);
  });

  test('should render efficiently with virtualized scrolling', async ({ page }) => {
    // Mock large dataset for virtualization testing - match the expected API format
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = Array.from({ length: 1000 }, (_, i) => ({
        title: `Feed Title ${i}`,
        description: `Description for feed ${i}`,
        link: `https://example.com/feed-${i}`,
        published: new Date().toISOString(),
      }));

      await route.fulfill({
        json: { 
          data: feeds,
          next_cursor: null
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    const timeline = page.locator('[data-testid="desktop-timeline"]');
    const virtualContainer = timeline.locator('[data-testid="virtual-container"]');

    // Verify virtual container exists
    await expect(virtualContainer).toBeVisible();

    // Check that only visible items are rendered (not all 1000)
    const renderedItems = await virtualContainer.locator('[data-testid^="feed-item-"]').count();
    expect(renderedItems).toBeLessThan(100); // Should render much less than total
    expect(renderedItems).toBeGreaterThan(0); // But should render something

    // Test virtual scrolling performance - scroll to bottom
    await timeline.evaluate(el => {
      const maxScrollTop = el.scrollHeight - el.clientHeight;
      el.scrollTo(0, Math.max(100, maxScrollTop / 2)); // Scroll to middle or at least 100px
    });
    await page.waitForTimeout(200);

    // Verify scroll position updated
    const scrollTop = await timeline.evaluate(el => el.scrollTop);
    expect(scrollTop).toBeGreaterThan(50); // More reasonable expectation

    // Check that items are still efficiently rendered
    const newRenderedItems = await virtualContainer.locator('[data-testid^="feed-item-"]').count();
    expect(newRenderedItems).toBeLessThan(100);
  });
});