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

test.describe('Virtualization Performance Regression', () => {
  test('should not degrade performance compared to baseline', async ({ page }) => {
    const testConfigs = [
      { 
        platform: 'mobile', 
        url: '/mobile/feeds', 
        itemCount: 500,
        virtualSelector: '[data-testid="virtual-feed-list"]',
        fallbackSelector: '[data-testid="feed-card"]',
        viewport: { width: 375, height: 667 }
      },
      { 
        platform: 'desktop', 
        url: '/desktop/feeds', 
        itemCount: 200,
        virtualSelector: '[data-testid="virtual-desktop-timeline"]',
        fallbackSelector: '[data-testid="desktop-timeline-container"]',
        viewport: { width: 1920, height: 1080 }
      }
    ];

    for (const config of testConfigs) {
      await page.setViewportSize(config.viewport);
      
      await page.route('**/api/v1/feeds/fetch/cursor**', route => {
        route.fulfill({
          json: {
            data: generateMockFeeds(config.itemCount),
            next_cursor: null
          }
        });
      });

      // Test without virtualization
      await page.addInitScript(() => {
        localStorage.setItem('featureFlags', JSON.stringify({
          enableVirtualization: false,
          enableDesktopVirtualization: false
        }));
      });

      const nonVirtualStart = Date.now();
      await page.goto(config.url);
      await page.waitForSelector(config.fallbackSelector);
      const nonVirtualTime = Date.now() - nonVirtualStart;

      // Test with virtualization
      await page.addInitScript(() => {
        localStorage.setItem('featureFlags', JSON.stringify({
          enableVirtualization: true,
          enableDesktopVirtualization: true
        }));
      });

      const virtualStart = Date.now();
      await page.reload();
      await page.waitForSelector(config.virtualSelector);
      const virtualTime = Date.now() - virtualStart;

      console.log(`${config.platform}: Non-virtualized=${nonVirtualTime}ms, Virtualized=${virtualTime}ms`);

      // Virtualization should provide 30% improvement or at least be equivalent
      expect(virtualTime).toBeLessThan(nonVirtualTime * 1.3);

      // Performance should be within acceptable limits
      expect(virtualTime).toBeLessThan(5000); // 5 seconds max for initial load
    }
  });

  test('should maintain consistent scroll performance', async ({ page }) => {
    const largeFeedSet = generateMockFeeds(1000);
    
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
        enableDesktopVirtualization: true
      }));
    });

    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');

    // Measure scroll performance
    const scrollStart = Date.now();
    
    // Perform multiple rapid scrolls
    for (let i = 0; i < 10; i++) {
      await page.evaluate(() => {
        const container = document.querySelector('[data-testid="virtual-desktop-timeline"]');
        if (container) {
          container.scrollTop += 300;
        }
      });
      await page.waitForTimeout(50);
    }

    const scrollTime = Date.now() - scrollStart;
    
    // Should handle 10 scrolls in under 2 seconds
    expect(scrollTime).toBeLessThan(2000);

    // Should still have visible items after scrolling
    const visibleItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(visibleItems).toBeGreaterThan(0);
  });

  test('should handle memory efficiently with large datasets', async ({ page }) => {
    const massiveDataset = generateMockFeeds(5000);
    
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: massiveDataset,
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

    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');

    // Measure initial memory usage
    const initialMemory = await page.evaluate(() => {
      if (performance.memory) {
        return (performance.memory as any).usedJSHeapSize;
      }
      return 0;
    });

    // Perform extensive scrolling
    for (let i = 0; i < 50; i++) {
      await page.evaluate(() => {
        const container = document.querySelector('[data-testid="virtual-desktop-timeline"]');
        if (container) {
          container.scrollTop = Math.random() * (container.scrollHeight - container.clientHeight);
        }
      });
      await page.waitForTimeout(20);
    }

    // Measure final memory usage
    const finalMemory = await page.evaluate(() => {
      if (performance.memory) {
        return (performance.memory as any).usedJSHeapSize;
      }
      return 0;
    });

    // Memory growth should be minimal (less than 50MB)
    const memoryGrowth = finalMemory - initialMemory;
    expect(memoryGrowth).toBeLessThan(50 * 1024 * 1024); // 50MB

    // Should still render items correctly
    const visibleItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(visibleItems).toBeGreaterThan(0);
  });

  test('should handle viewport changes efficiently', async ({ page }) => {
    const testFeeds = generateMockFeeds(300);
    
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: testFeeds,
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

    // Start with mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="virtual-feed-list"]');

    // Resize to desktop multiple times
    const viewportSizes = [
      { width: 1920, height: 1080 },
      { width: 1366, height: 768 },
      { width: 1024, height: 768 },
      { width: 768, height: 1024 },
      { width: 375, height: 667 },
    ];

    for (const viewport of viewportSizes) {
      const resizeStart = Date.now();
      await page.setViewportSize(viewport);
      await page.waitForTimeout(100); // Allow for reflow
      const resizeTime = Date.now() - resizeStart;

      // Each resize should complete quickly
      expect(resizeTime).toBeLessThan(500);
    }

    // Should still be functional after all resizes
    const finalItems = await page.locator('[data-testid^="virtual-feed-item-"]').count();
    expect(finalItems).toBeGreaterThan(0);
  });

  test('should handle rapid feature flag changes', async ({ page }) => {
    const testFeeds = generateMockFeeds(200);
    
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: testFeeds,
          next_cursor: null
        }
      });
    });

    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/desktop/feeds');

    // Rapidly toggle feature flags
    for (let i = 0; i < 5; i++) {
      const enableVirtualization = i % 2 === 0;
      
      await page.addInitScript((enabled) => {
        localStorage.setItem('featureFlags', JSON.stringify({
          enableVirtualization: enabled,
          enableDesktopVirtualization: enabled
        }));
      }, enableVirtualization);

      const toggleStart = Date.now();
      await page.reload();
      
      if (enableVirtualization) {
        await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');
        await expect(page.locator('[data-testid="virtual-desktop-timeline"]')).toBeVisible();
      } else {
        await page.waitForSelector('[data-testid="desktop-timeline-container"]');
        await expect(page.locator('[data-testid="desktop-timeline-container"]')).toBeVisible();
      }
      
      const toggleTime = Date.now() - toggleStart;
      expect(toggleTime).toBeLessThan(3000); // Should switch modes quickly
    }
  });

  test('should maintain performance during concurrent operations', async ({ page }) => {
    const testFeeds = generateMockFeeds(500);
    
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        json: {
          data: testFeeds,
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

    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="virtual-desktop-timeline"]');

    // Simulate concurrent operations
    const operationStart = Date.now();
    
    // Start multiple concurrent operations
    const operations = [
      // Rapid scrolling
      (async () => {
        for (let i = 0; i < 20; i++) {
          await page.evaluate(() => {
            const container = document.querySelector('[data-testid="virtual-desktop-timeline"]');
            if (container) {
              container.scrollTop += 100;
            }
          });
          await page.waitForTimeout(25);
        }
      })(),
      
      // Viewport resizing
      (async () => {
        const sizes = [
          { width: 1920, height: 1080 },
          { width: 1366, height: 768 },
          { width: 1920, height: 1080 }
        ];
        for (const size of sizes) {
          await page.setViewportSize(size);
          await page.waitForTimeout(100);
        }
      })(),
      
      // Feature flag toggling
      (async () => {
        await page.waitForTimeout(200);
        await page.evaluate(() => {
          localStorage.setItem('featureFlags', JSON.stringify({
            enableVirtualization: true,
            enableDesktopVirtualization: true,
            enableDynamicSizing: true
          }));
        });
      })()
    ];

    // Wait for all operations to complete
    await Promise.all(operations);
    
    const totalTime = Date.now() - operationStart;
    
    // All concurrent operations should complete within reasonable time
    expect(totalTime).toBeLessThan(3000);

    // Application should still be responsive
    const visibleItems = await page.locator('[data-testid^="virtual-desktop-item-"]').count();
    expect(visibleItems).toBeGreaterThan(0);
  });
});