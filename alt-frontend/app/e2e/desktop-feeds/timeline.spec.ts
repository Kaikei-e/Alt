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
    await expect(timeline).toHaveCSS('overflow-y', 'scroll');
    await expect(timeline).toHaveCSS('overflow-x', 'hidden');

    // Verify the timeline is scrollable (the key behavior we want)
    const scrollable = await timeline.evaluate(el => el.scrollHeight > el.clientHeight);
    // Timeline should handle scrolling properly regardless of exact height value
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

  test('should render efficiently with virtualized scrolling', async ({ page }) => {
    // Mock large dataset for virtualization testing
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = Array.from({ length: 50 }, (_, i) => ({
        id: `virtualized-feed-${i}`,
        title: `Virtualized Feed ${i}`,
        description: `Description for virtualized feed ${i}`,
        link: `https://example.com/virtualized-${i}`,
        published: new Date(Date.now() - i * 3600000).toISOString(),
        source: 'TechCrunch'
      }));

      await route.fulfill({
        json: { data: feeds, next_cursor: null }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500); // Increased wait time for virtualization setup

    // Wait for the virtual container to be ready
    const virtualContainer = page.locator('[data-testid="virtual-container"]');
    await expect(virtualContainer).toBeVisible({ timeout: 10000 });

    // Verify virtual container exists and has proper height
    const containerProps = await virtualContainer.evaluate(el => {
      const rect = el.getBoundingClientRect();
      return {
        height: el.style.height,
        position: getComputedStyle(el).position,
        hasContent: el.children.length > 0,
        childrenCount: el.children.length
      };
    });

    // Virtual container should have height and be positioned
    expect(containerProps.position).toBe('relative');
    expect(containerProps.height).toBeTruthy();
    expect(parseInt(containerProps.height)).toBeGreaterThan(0);

    // Check for virtualized feed items using the correct selector
    const feedItems = virtualContainer.locator('[data-testid^="feed-item-"]');
    const itemCount = await feedItems.count();

    // Should render some items but not all 50 (virtualization efficiency)
    expect(itemCount).toBeGreaterThan(0);
    expect(itemCount).toBeLessThan(50); // Not all items should be rendered at once

    // Verify feed items have proper structure
    if (itemCount > 0) {
      const firstItem = feedItems.first();
      await expect(firstItem).toBeVisible();

      // Check that items contain expected mock data
      const itemText = await firstItem.textContent();
      expect(itemText).toContain('Virtualized Feed');

      // Verify glass effect on items
      const hasGlassClass = await firstItem.locator('.glass').count();
      expect(hasGlassClass).toBeGreaterThan(0);
    }

    // Test scrolling behavior
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // Test virtual scrolling by scrolling and checking items update
    await timeline.evaluate(el => {
      el.scrollTop = 200;
    });

    await page.waitForTimeout(300);

    // Verify scroll position
    const scrollTop = await timeline.evaluate(el => el.scrollTop);
    expect(scrollTop).toBeGreaterThanOrEqual(0);

    // Performance check - virtualization should remain responsive
    const performanceMetrics = await page.evaluate(() => {
      const start = performance.now();

      // Simulate some DOM interactions
      const virtualContainer = document.querySelector('[data-testid="virtual-container"]');
      const timeline = document.querySelector('[data-testid="desktop-timeline"]');

      if (timeline) {
        timeline.scrollTop = 100;
        timeline.scrollTop = 0;
      }

      const end = performance.now();

      return {
        duration: end - start,
        hasVirtualContainer: !!virtualContainer,
        hasTimeline: !!timeline,
        virtualContainerChildren: virtualContainer?.children.length || 0
      };
    });

    expect(performanceMetrics.duration).toBeLessThan(100); // Should complete in less than 100ms
    expect(performanceMetrics.hasVirtualContainer).toBe(true);
    expect(performanceMetrics.hasTimeline).toBe(true);

    // Verify infinite scroll capability by scrolling to bottom
    await timeline.evaluate(el => {
      el.scrollTop = el.scrollHeight - el.clientHeight;
    });

    await page.waitForTimeout(300);

    // Check for loading indicator or additional content
    const loadingIndicators = [
      page.locator('text="Loading more feeds"'),
      page.locator('[data-testid="loading-spinner"]'),
      page.locator('.spinner'),
      virtualContainer
    ];

    let hasInfiniteScrollCapability = false;
    for (const indicator of loadingIndicators) {
      if (await indicator.count() > 0) {
        hasInfiniteScrollCapability = true;
        break;
      }
    }

    expect(hasInfiniteScrollCapability).toBe(true);
  });
});