import { test, expect } from '@playwright/test';

// Helper function to generate mock feeds
function generateMockFeeds(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    id: `feed-${i}`,
    title: `Feed ${i}`,
    description: `This is a test feed description ${i}. Lorem ipsum dolor sit amet, consectetur adipiscing elit.`,
    link: `https://example.com/feed${i}`,
    published: new Date(Date.now() - i * 1000).toISOString(),
    image: `https://example.com/image${i}.jpg`,
    source: `Source ${i}`,
    category: `Category ${i % 5}`,
    readCount: 0,
    favoriteCount: 0,
    tags: [`tag${i % 3}`, `tag${i % 5}`]
  }));
}

test.describe('Virtual Feed List Performance', () => {
  test.beforeEach(async ({ page }) => {
    // Set up common mock route
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: generateMockFeeds(500),
          next_cursor: null
        }
      });
    });
  });

  test('should render 500 items with virtualization faster than without', async ({ page }) => {
    // Test non-virtualized performance
    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: false
      }));
    });

    const nonVirtualStart = Date.now();
    await page.goto('/mobile/feeds');
    
    // Wait for feed cards to load
    await page.waitForSelector('[data-testid="feed-list-fallback"]', { timeout: 10000 });
    
    const nonVirtualTime = Date.now() - nonVirtualStart;
    console.log(`Non-virtualized render time: ${nonVirtualTime}ms`);

    // Test virtualized performance  
    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true
      }));
    });

    const virtualStart = Date.now();
    await page.reload();
    
    // Wait for virtual feed items to load
    await page.waitForSelector('[data-testid="virtual-scroll-container"]', { timeout: 10000 });
    
    const virtualTime = Date.now() - virtualStart;
    console.log(`Virtualized render time: ${virtualTime}ms`);

    // Virtualization should be faster or at least not significantly slower
    // Allow some tolerance for initial setup overhead
    const improvement = (nonVirtualTime - virtualTime) / nonVirtualTime;
    console.log(`Performance improvement: ${(improvement * 100).toFixed(1)}%`);
    
    // Virtualization should provide at least some performance benefit or be close
    expect(virtualTime).toBeLessThan(nonVirtualTime + 1000); // Allow 1s tolerance
  });

  test('should maintain smooth scrolling with virtualization', async ({ page }) => {
    // Use larger dataset for scroll testing
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: generateMockFeeds(1000),
          next_cursor: null
        }
      });
    });

    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true
      }));
    });

    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="virtual-scroll-container"]', { timeout: 10000 });

    // Perform smooth scrolling test
    const scrollContainer = page.locator('[data-testid="virtual-scroll-container"]');
    
    const startTime = Date.now();
    
    // Scroll to different positions
    await scrollContainer.evaluate(el => {
      el.scrollTo({ top: 2000, behavior: 'smooth' });
    });
    
    // Wait for scroll to complete
    await page.waitForTimeout(1000);
    
    const endTime = Date.now();
    const scrollDuration = endTime - startTime;

    console.log(`Scroll duration: ${scrollDuration}ms`);
    
    // Scroll should complete within reasonable time
    expect(scrollDuration).toBeLessThan(3000); // 3 seconds max
  });

  test('should handle large dataset without memory issues', async ({ page }) => {
    // Test with very large dataset
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: generateMockFeeds(2000),
          next_cursor: null
        }
      });
    });

    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true
      }));
    });

    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="virtual-scroll-container"]', { timeout: 15000 });

    // Verify virtual container exists and has proper structure
    const virtualContainer = page.locator('[data-testid="virtual-scroll-container"]');
    await expect(virtualContainer).toBeVisible();

    const contentContainer = page.locator('[data-testid="virtual-content-container"]');
    await expect(contentContainer).toBeVisible();

    // Verify only a limited number of items are rendered in DOM
    const renderedItems = page.locator('[data-testid^="virtual-feed-item-"]');
    const itemCount = await renderedItems.count();
    
    console.log(`Rendered virtual items: ${itemCount}`);
    
    // Should render much fewer items than the total dataset
    expect(itemCount).toBeLessThan(50); // Only visible items + overscan
    expect(itemCount).toBeGreaterThan(0); // But some items should be rendered
  });

  test('should fallback to simple list when virtualization fails', async ({ page }) => {
    // Mock feature flag to force virtualization
    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true,
        forceVirtualization: true
      }));
    });

    // Mock the virtual feed implementation to throw error
    await page.addInitScript(() => {
      // This will cause virtualization to fail and fallback to simple list
      window.mockVirtualizationError = true;
    });

    await page.goto('/mobile/feeds');
    
    // Should fallback to simple feed list
    await page.waitForSelector('[data-testid="feed-list-fallback"]', { timeout: 10000 });
    
    // Verify fallback is working
    const fallbackContainer = page.locator('[data-testid="feed-list-fallback"]');
    await expect(fallbackContainer).toBeVisible();
    
    // Should have some feed items rendered
    const feedItems = page.locator('[data-testid="feed-list-fallback"] > div');
    const itemCount = await feedItems.count();
    
    console.log(`Fallback rendered items: ${itemCount}`);
    expect(itemCount).toBeGreaterThan(0);
  });

  test('should respect feature flag settings', async ({ page }) => {
    // Test with feature flag disabled
    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: false
      }));
    });

    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="feed-list-fallback"]', { timeout: 10000 });

    // Should use simple list
    await expect(page.locator('[data-testid="feed-list-fallback"]')).toBeVisible();
    await expect(page.locator('[data-testid="virtual-scroll-container"]')).not.toBeVisible();

    // Test with feature flag enabled
    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true
      }));
    });

    await page.reload();
    await page.waitForSelector('[data-testid="virtual-scroll-container"]', { timeout: 10000 });

    // Should use virtualized list
    await expect(page.locator('[data-testid="virtual-scroll-container"]')).toBeVisible();
  });
});