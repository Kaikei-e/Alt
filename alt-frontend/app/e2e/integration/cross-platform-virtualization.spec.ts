import { test, expect } from '@playwright/test';

// Mock feed generator
const generateMockFeeds = (count: number) => {
  return Array.from({ length: count }, (_, i) => ({
    id: `feed-${i}`,
    title: `Feed Title ${i}`,
    description: `Description for feed ${i}`,
    link: `https://example.com/feed-${i}`,
    published: new Date().toISOString(),
  }));
};

const generateDesktopMockFeeds = (count: number) => {
  return Array.from({ length: count }, (_, i) => ({
    id: `desktop-feed-${i}`,
    title: `Desktop Feed Title ${i}`,
    description: `Detailed description for desktop feed ${i} with more comprehensive content`,
    link: `https://example.com/desktop-feed-${i}`,
    published: new Date().toISOString(),
  }));
};

test.describe('Cross-Platform Virtualization Integration', () => {
  test('should work consistently across mobile and desktop', async ({ page }) => {
    const largeFeedSet = generateMockFeeds(300);
    
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: largeFeedSet,
          next_cursor: null
        }
      });
    });

    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true,
        enableDesktopVirtualization: true,
        enableDynamicSizing: true
      }));
    });

    // Test mobile virtualization
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="virtual-feed-list"]');
    
    const mobileItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    expect(mobileItems).toBeGreaterThan(0);
    expect(mobileItems).toBeLessThan(50);

    // Test desktop virtualization
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    const desktopItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(desktopItems).toBeGreaterThan(0);
    expect(desktopItems).toBeLessThan(20); // Desktop cards are larger, so fewer items

    // Test scrolling behavior on both platforms
    await page.evaluate(() => {
      const container = document.querySelector('[data-testid="virtual-desktop-timeline"]');
      if (container) {
        container.scrollTop = container.scrollHeight / 2;
      }
    });

    await page.waitForTimeout(500);
    
    // Verify scrolling works correctly
    const afterScrollItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(afterScrollItems).toBeGreaterThan(0);
  });

  test('should handle feature flag changes consistently', async ({ page }) => {
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: generateMockFeeds(200),
          next_cursor: null
        }
      });
    });

    // Test with virtualization enabled
    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true,
        enableDesktopVirtualization: true
      }));
    });

    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    // Change to virtualization disabled
    await page.evaluate(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: false,
        enableDesktopVirtualization: false
      }));
    });

    await page.reload();
    await page.waitForLoadState('networkidle');
    
    // Should fallback to regular timeline
    await expect(page.locator('[data-testid="virtual-desktop-timeline"]')).not.toBeVisible();
    await expect(page.locator('[data-testid="desktop-timeline-container"]')).toBeVisible();
  });

  test('should maintain performance across viewports', async ({ page }) => {
    const testConfigs = [
      { 
        viewport: { width: 375, height: 667 }, 
        url: '/mobile/feeds',
        selector: '[data-testid="virtual-feed-list"]',
        itemSelector: '[data-testid^="virtual-feed-item-"]',
        platform: 'mobile',
        maxItems: 50
      },
      { 
        viewport: { width: 1920, height: 1080 }, 
        url: '/desktop/feeds',
        selector: '[data-testid="virtual-desktop-timeline"]',
        itemSelector: '[data-testid^="virtual-desktop-item-"]',
        platform: 'desktop',
        maxItems: 20
      }
    ];

    for (const config of testConfigs) {
      await page.route('**/api/v1/feeds/fetch/cursor**', route => {
        route.fulfill({
          json: {
            data: config.platform === 'mobile' ? generateMockFeeds(300) : generateDesktopMockFeeds(200),
            next_cursor: null
          }
        });
      });

      await page.addInitScript(() => {
        localStorage.setItem('featureFlags', JSON.stringify({
          enableVirtualization: true,
          enableDesktopVirtualization: true
        }));
      });

      await page.setViewportSize(config.viewport);
      
      const startTime = Date.now();
      await page.goto(config.url);
      await page.waitForSelector(config.selector);
      const loadTime = Date.now() - startTime;

      // Performance requirement: load within 3 seconds
      expect(loadTime).toBeLessThan(3000);

      // Check correct number of items are rendered
      const renderedItems = await page.locator(config.itemSelector).count();
      expect(renderedItems).toBeGreaterThan(0);
      expect(renderedItems).toBeLessThan(config.maxItems);
    }
  });

  test('should handle large datasets efficiently across platforms', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true,
        enableDesktopVirtualization: true
      }));
    });

    const largeDataset = generateMockFeeds(1000);
    
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: largeDataset,
          next_cursor: null
        }
      });
    });

    // Test mobile with large dataset
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="virtual-feed-list"]');

    const mobileStartTime = Date.now();
    const mobileItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    const mobileTime = Date.now() - mobileStartTime;

    expect(mobileItems).toBeGreaterThan(0);
    expect(mobileItems).toBeLessThan(50); // Only visible items rendered
    expect(mobileTime).toBeLessThan(1000); // Quick rendering

    // Test desktop with large dataset
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');

    const desktopStartTime = Date.now();
    const desktopItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    const desktopTime = Date.now() - desktopStartTime;

    expect(desktopItems).toBeGreaterThan(0);
    expect(desktopItems).toBeLessThan(20); // Desktop cards are larger
    expect(desktopTime).toBeLessThan(1000); // Quick rendering

    // Test scrolling performance
    for (let i = 0; i < 5; i++) {
      await page.evaluate(() => {
        const container = document.querySelector('[data-testid="virtual-desktop-timeline"]');
        if (container) {
          container.scrollTop += 500;
        }
      });
      await page.waitForTimeout(100);
    }

    // Should still be responsive after scrolling
    const afterScrollItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(afterScrollItems).toBeGreaterThan(0);
  });

  test('should handle empty states consistently', async ({ page }) => {
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: [],
          next_cursor: null
        }
      });
    });

    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true,
        enableDesktopVirtualization: true
      }));
    });

    // Test mobile empty state
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="virtual-feed-empty-state"]');
    
    await expect(page.locator('[data-testid="virtual-feed-empty-state"]')).toBeVisible();
    await expect(page.getByText('No feeds available')).toBeVisible();

    // Test desktop empty state
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-empty-state"]');
    
    await expect(page.locator('[data-testid="virtual-desktop-empty-state"]')).toBeVisible();
    await expect(page.getByText('No feeds available')).toBeVisible();
  });

  test('should handle error recovery gracefully', async ({ page }) => {
    let requestCount = 0;
    
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      requestCount++;
      if (requestCount === 1) {
        // First request fails
        route.fulfill({
          status: 500,
          json: { error: 'Server error' }
        });
      } else {
        // Subsequent requests succeed
        route.fulfill({
          json: {
            data: generateMockFeeds(100),
            next_cursor: null
          }
        });
      }
    });

    await page.addInitScript(() => {
      localStorage.setItem('featureFlags', JSON.stringify({
        enableVirtualization: true,
        enableDesktopVirtualization: true
      }));
    });

    await page.goto('/desktop/feeds');
    
    // Should show error initially
    await expect(page.getByText('Failed to load feeds')).toBeVisible();
    
    // Should recover after retry
    await page.getByText('Retry').click();
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
    
    // Should show virtualized content after recovery
    const items = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(items).toBeGreaterThan(0);
  });
});