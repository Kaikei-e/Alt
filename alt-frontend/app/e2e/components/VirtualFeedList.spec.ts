import { test, expect } from '@playwright/test';

test.describe('VirtualFeedList Component - Performance Tests', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to mobile feeds page
    await page.goto('/mobile/feeds');
  });

  test('should render virtual feed list with visible items only', async ({ page }) => {
    // Wait for the virtual container to be present
    await page.waitForSelector('[data-testid="virtual-feed-list"]');
    
    const virtualContainer = page.locator('[data-testid="virtual-feed-list"]');
    await expect(virtualContainer).toBeVisible();

    // Check that only visible items are rendered in DOM
    const renderedItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    
    // Should render only visible items + overscan (estimated 10-15 items for viewport)
    expect(renderedItems).toBeGreaterThan(0);
    expect(renderedItems).toBeLessThan(25); // Maximum expected with overscan
  });

  test('should handle scrolling and render new items dynamically', async ({ page }) => {
    // Wait for virtual list to load
    await page.waitForSelector('[data-testid="virtual-feed-list"]');
    
    // Get initial rendered items
    const initialItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    
    // Scroll down to trigger new items
    await page.locator('[data-testid="virtual-feed-list"]').scrollIntoView();
    await page.keyboard.press('PageDown');
    await page.keyboard.press('PageDown');
    
    // Wait for scroll to complete
    await page.waitForTimeout(500);
    
    // Check that scroll position changed and new items are rendered
    const scrollTop = await page.evaluate(() => {
      const element = document.querySelector('[data-testid="virtual-feed-list"]');
      return element?.scrollTop || 0;
    });
    
    expect(scrollTop).toBeGreaterThan(0);
    
    // Should still maintain reasonable DOM size
    const newItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    expect(newItems).toBeLessThan(30);
  });

  test('should maintain performance with large datasets', async ({ page }) => {
    // Navigate to page and wait for load
    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="virtual-feed-list"]');
    
    // Measure initial performance
    const startTime = Date.now();
    
    // Perform multiple scroll operations
    for (let i = 0; i < 5; i++) {
      await page.keyboard.press('PageDown');
      await page.waitForTimeout(100);
    }
    
    const endTime = Date.now();
    const duration = endTime - startTime;
    
    // Should complete scrolling operations within reasonable time
    expect(duration).toBeLessThan(2000); // 2 seconds for 5 scroll operations
    
    // Check that DOM size is still reasonable
    const finalItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    expect(finalItems).toBeLessThan(30);
  });

  test('should handle empty state gracefully', async ({ page }) => {
    // Mock empty response
    await page.route('**/feeds/cursor*', route => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          feeds: [],
          hasMore: false,
          cursor: null
        })
      });
    });
    
    await page.goto('/mobile/feeds');
    
    // Should show empty state instead of virtual list
    await expect(page.locator('[data-testid="virtual-feed-list"]')).not.toBeVisible();
    await expect(page.getByText('No feeds available')).toBeVisible();
  });

  test('should work across different viewport sizes', async ({ page }) => {
    // Test mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="virtual-feed-list"]');
    
    let mobileItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    expect(mobileItems).toBeGreaterThan(0);
    
    // Test tablet viewport
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.reload();
    await page.waitForSelector('[data-testid="virtual-feed-list"]');
    
    let tabletItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    expect(tabletItems).toBeGreaterThan(0);
    
    // Both should maintain reasonable DOM size
    expect(mobileItems).toBeLessThan(25);
    expect(tabletItems).toBeLessThan(30);
  });
});