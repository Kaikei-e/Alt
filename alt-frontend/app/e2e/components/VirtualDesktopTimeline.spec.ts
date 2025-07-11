import { test, expect } from '@playwright/test';

test.describe('VirtualDesktopTimeline Component - Performance Tests', () => {
  test.beforeEach(async ({ page }) => {
    // Mock API endpoints to prevent network errors
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [
            {
              title: 'Test Feed 1',
              description: 'Description for test feed 1',
              link: 'https://example.com/feed1',
              published: '2024-01-01T12:00:00Z'
            },
            {
              title: 'Test Feed 2',
              description: 'Description for test feed 2',
              link: 'https://example.com/feed2',
              published: '2024-01-02T12:00:00Z'
            }
          ],
          next_cursor: null
        })
      });
    });

    // Mock other API endpoints that might be called
    await page.route('**/api/v1/feeds/stats**', route => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          feed_amount: { amount: 42 },
          summarized_feed: { amount: 28 }
        })
      });
    });

    await page.route('**/api/v1/health**', route => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'ok' })
      });
    });

    // Navigate to desktop feeds page
    await page.goto('/desktop/feeds');
  });

  test('should render virtual desktop timeline with visible items only', async ({ page }) => {
    // Wait for the page to load and check for presence of feed cards
    await page.waitForLoadState('networkidle');
    
    // Check for feed cards (actual implementation uses LazyDesktopTimeline)
    const feedCards = page.locator('[data-testid="feed-card"]');
    await expect(feedCards.first()).toBeVisible({ timeout: 10000 });

    // Check that feed cards are rendered
    const renderedItems = await feedCards.count();
    
    // Should render feed cards based on mock data
    expect(renderedItems).toBeGreaterThan(0);
    expect(renderedItems).toBeLessThanOrEqual(2); // Based on mock data
  });

  test('should handle scrolling efficiently with larger desktop cards', async ({ page }) => {
    // Wait for page to load completely
    await page.waitForLoadState('networkidle');
    
    // Wait for feed cards to be present
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible({ timeout: 10000 });
    
    // Get initial rendered items
    const initialItems = await page.locator('[data-testid="feed-card"]').count();
    
    // Scroll down within the page
    await page.keyboard.press('PageDown');
    await page.keyboard.press('PageDown');
    
    // Wait for scroll to complete
    await page.waitForTimeout(500);
    
    // Check that scroll position changed
    const scrollTop = await page.evaluate(() => {
      return window.scrollY;
    });
    
    expect(scrollTop).toBeGreaterThan(0);
    
    // Should still maintain reasonable DOM size for desktop
    const newItems = await page.locator('[data-testid="feed-card"]').count();
    expect(newItems).toBeLessThanOrEqual(2); // Based on mock data
  });

  test('should maintain performance with desktop-sized cards', async ({ page }) => {
    // Navigate to page and wait for load
    await page.goto('/desktop/feeds');
    await page.waitForLoadState('networkidle');
    
    // Wait for feed cards to be present
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible({ timeout: 10000 });
    
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
    const finalItems = await page.locator('[data-testid="feed-card"]').count();
    expect(finalItems).toBeLessThanOrEqual(2); // Based on mock data
  });

  test('should handle desktop feed interactions correctly', async ({ page }) => {
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    // Find first feed card within the virtual desktop timeline
    const firstFeedCard = page.locator('[data-testid="virtual-desktop-timeline"]').locator('.glass').first();
    await expect(firstFeedCard).toBeVisible();
    
    // Test buttons within the feed card
    const markAsReadButton = firstFeedCard.locator('button', { hasText: 'Mark as Read' });
    if (await markAsReadButton.count() > 0) {
      await markAsReadButton.click();
      await page.waitForTimeout(200);
    }
    
    // Test favorite button
    const favoriteButton = firstFeedCard.locator('button[aria-label*="favorite"]');
    if (await favoriteButton.count() > 0) {
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
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [],
          next_cursor: null
        })
      });
    });
    
    await page.goto('/desktop/feeds');
    
    // Should show empty state
    await expect(page.getByText('No feeds available')).toBeVisible();
    await expect(page.getByText('Your feed will appear here once you subscribe to sources')).toBeVisible();
  });
});