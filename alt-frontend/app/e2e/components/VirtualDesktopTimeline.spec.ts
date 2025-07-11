import { test, expect } from '@playwright/test';

test.describe('VirtualDesktopTimeline Component - Performance Tests', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to desktop feeds page
    await page.goto('/desktop/feeds');
  });

  test('should render virtual desktop timeline with visible items only', async ({ page }) => {
    // Wait for the virtual container to be present
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    const virtualContainer = page.locator('[data-testid="virtual-desktop-timeline"]');
    await expect(virtualContainer).toBeVisible();

    // Check that only visible items are rendered in DOM
    const renderedItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    
    // Should render only visible items + overscan (estimated 6-10 items for desktop viewport)
    expect(renderedItems).toBeGreaterThan(0);
    expect(renderedItems).toBeLessThan(15); // Maximum expected with overscan
  });

  test('should handle scrolling efficiently with larger desktop cards', async ({ page }) => {
    // Wait for virtual list to load
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    // Get initial rendered items
    const initialItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    
    // Scroll down to trigger new items
    await page.locator('[data-testid="virtual-desktop-timeline"]').scrollIntoView();
    await page.keyboard.press('PageDown');
    await page.keyboard.press('PageDown');
    
    // Wait for scroll to complete
    await page.waitForTimeout(500);
    
    // Check that scroll position changed
    const scrollTop = await page.evaluate(() => {
      const element = document.querySelector('[data-testid="virtual-desktop-timeline"]');
      return element?.scrollTop || 0;
    });
    
    expect(scrollTop).toBeGreaterThan(0);
    
    // Should still maintain reasonable DOM size for desktop
    const newItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(newItems).toBeLessThan(20);
  });

  test('should maintain performance with desktop-sized cards', async ({ page }) => {
    // Navigate to page and wait for load
    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    // Measure initial performance
    const startTime = Date.now();
    
    // Perform multiple scroll operations
    for (let i = 0; i < 3; i++) {
      await page.keyboard.press('PageDown');
      await page.waitForTimeout(150);
    }
    
    const endTime = Date.now();
    const duration = endTime - startTime;
    
    // Should complete scrolling operations within reasonable time
    expect(duration).toBeLessThan(1500); // 1.5 seconds for 3 scroll operations
    
    // Check that DOM size is still reasonable
    const finalItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(finalItems).toBeLessThan(20);
  });

  test('should handle desktop feed interactions correctly', async ({ page }) => {
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    // Find first feed card
    const firstFeedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    await expect(firstFeedCard).toBeVisible();
    
    // Test mark as read button
    const markAsReadButton = firstFeedCard.locator('button', { hasText: 'Mark as Read' });
    if (await markAsReadButton.isVisible()) {
      await markAsReadButton.click();
      await page.waitForTimeout(200);
    }
    
    // Test favorite button
    const favoriteButton = firstFeedCard.locator('button[aria-label*="favorite"]');
    if (await favoriteButton.isVisible()) {
      await favoriteButton.click();
      await page.waitForTimeout(200);
    }
    
    // Verify DOM size hasn't exploded after interactions
    const items = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(items).toBeLessThan(20);
  });

  test('should work with different desktop viewport sizes', async ({ page }) => {
    // Test desktop viewport (1280x720)
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    let desktopItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(desktopItems).toBeGreaterThan(0);
    
    // Test large desktop viewport (1920x1080)
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.reload();
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    let largeDesktopItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(largeDesktopItems).toBeGreaterThan(0);
    
    // Both should maintain reasonable DOM size
    expect(desktopItems).toBeLessThan(15);
    expect(largeDesktopItems).toBeLessThan(20);
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
    
    await page.goto('/desktop/feeds');
    
    // Should show empty state
    await expect(page.getByText('No feeds available')).toBeVisible();
    await expect(page.getByText('Your feed will appear here once you subscribe to sources')).toBeVisible();
  });
});