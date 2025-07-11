import { test, expect } from '@playwright/test';
import { generateMockFeeds } from '../helpers/mockApi';

test.describe('Feed List Performance Baseline', () => {
  // Performance measurement tests for different item counts
  [50, 100, 200, 500, 1000].forEach(itemCount => {
    test(`should render ${itemCount} items within performance threshold`, async ({ page }) => {
      // Mock API with specified item count
      await page.route('**/api/v1/feeds/fetch/cursor**', route => {
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: generateMockFeeds(itemCount).map(feed => ({
              title: feed.title,
              description: feed.description,
              link: feed.link,
              published: feed.published,
            })),
            next_cursor: null
          })
        });
      });

      // Measure initial render time
      const startTime = Date.now();
      await page.goto('/mobile/feeds');
      await page.waitForSelector('[data-testid="feed-card"]', { timeout: 15000 });
      const renderTime = Date.now() - startTime;

      // Measure scroll performance
      const scrollStart = Date.now();
      await page.evaluate(() => {
        window.scrollTo(0, document.body.scrollHeight);
      });
      await page.waitForTimeout(100); // Allow scroll to complete
      const scrollTime = Date.now() - scrollStart;

      // Log performance metrics for analysis
      console.log(`${itemCount} items - Render: ${renderTime}ms, Scroll: ${scrollTime}ms`);

      // Performance thresholds based on TASK1 specification
      expect(renderTime).toBeLessThan(3000); // 3 seconds maximum
      expect(scrollTime).toBeLessThan(1000); // 1 second maximum

      // Verify that items are actually rendered
      const feedCards = page.locator('[data-testid="feed-card"]');
      const renderedCount = await feedCards.count();
      expect(renderedCount).toBeGreaterThan(0);

      // For larger datasets, expect some performance characteristics
      if (itemCount >= 500) {
        console.warn(`Large dataset (${itemCount} items) - Monitor for performance degradation`);
      }
    });
  });

  test('should track memory usage across different item counts', async ({ page }) => {
    const memoryResults = [];
    
    for (const itemCount of [50, 100, 200, 500]) {
      await page.route('**/api/v1/feeds/fetch/cursor**', route => {
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: generateMockFeeds(itemCount).map(feed => ({
              title: feed.title,
              description: feed.description,
              link: feed.link,
              published: feed.published,
            })),
            next_cursor: null
          })
        });
      });

      await page.goto('/mobile/feeds');
      await page.waitForSelector('[data-testid="feed-card"]');

      // Measure memory usage (if available)
      const memoryInfo = await page.evaluate(() => {
        // @ts-ignore - performance.memory is available in Chrome
        return (performance as any).memory;
      });

      if (memoryInfo) {
        memoryResults.push({
          itemCount,
          usedJSHeapSize: memoryInfo.usedJSHeapSize,
          totalJSHeapSize: memoryInfo.totalJSHeapSize
        });

        console.log(`${itemCount} items - Memory: ${Math.round(memoryInfo.usedJSHeapSize / 1024 / 1024)}MB`);
      }
    }

    // Basic memory growth analysis
    if (memoryResults.length >= 2) {
      const firstResult = memoryResults[0];
      const lastResult = memoryResults[memoryResults.length - 1];
      const growthRate = lastResult.usedJSHeapSize / firstResult.usedJSHeapSize;
      
      console.log(`Memory growth rate: ${growthRate.toFixed(2)}x from ${firstResult.itemCount} to ${lastResult.itemCount} items`);
      
      // Memory growth should be reasonable (less than 2x for 10x items)
      expect(growthRate).toBeLessThan(2.0);
    }
  });

  test('should measure DOM node count scaling', async ({ page }) => {
    const domResults = [];

    for (const itemCount of [50, 100, 200]) {
      await page.route('**/api/v1/feeds/fetch/cursor**', route => {
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            data: generateMockFeeds(itemCount).map(feed => ({
              title: feed.title,
              description: feed.description,
              link: feed.link,
              published: feed.published,
            })),
            next_cursor: null
          })
        });
      });

      await page.goto('/mobile/feeds');
      await page.waitForSelector('[data-testid="feed-card"]');

      // Count DOM nodes
      const domNodeCount = await page.evaluate(() => {
        return document.querySelectorAll('*').length;
      });

      // Count feed cards specifically
      const feedCardCount = await page.locator('[data-testid="feed-card"]').count();

      domResults.push({
        itemCount,
        domNodeCount,
        feedCardCount,
        nodesPerCard: domNodeCount / feedCardCount
      });

      console.log(`${itemCount} items - DOM nodes: ${domNodeCount}, Cards: ${feedCardCount}, Nodes/Card: ${Math.round(domNodeCount / feedCardCount)}`);
    }

    // Verify DOM scaling is reasonable
    if (domResults.length >= 2) {
      const avgNodesPerCard = domResults.reduce((sum, result) => sum + result.nodesPerCard, 0) / domResults.length;
      console.log(`Average DOM nodes per card: ${Math.round(avgNodesPerCard)}`);
      
      // Each feed card should have a reasonable DOM footprint
      expect(avgNodesPerCard).toBeLessThan(50); // Arbitrary reasonable limit
    }
  });

  test('should verify scroll performance characteristics', async ({ page }) => {
    const itemCount = 500; // Medium-large dataset
    
    await page.route('**/api/v1/feeds/fetch/cursor**', route => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: generateMockFeeds(itemCount).map(feed => ({
            title: feed.title,
            description: feed.description,
            link: feed.link,
            published: feed.published,
          })),
          next_cursor: null
        })
      });
    });

    await page.goto('/mobile/feeds');
    await page.waitForSelector('[data-testid="feed-card"]');

    // Test rapid scrolling
    const scrollTimes = [];
    
    for (let i = 0; i < 5; i++) {
      const scrollStart = Date.now();
      await page.evaluate((scrollPosition) => {
        window.scrollTo(0, scrollPosition);
      }, i * 1000);
      await page.waitForTimeout(100);
      const scrollTime = Date.now() - scrollStart;
      scrollTimes.push(scrollTime);
    }

    const avgScrollTime = scrollTimes.reduce((sum, time) => sum + time, 0) / scrollTimes.length;
    console.log(`Average scroll time: ${avgScrollTime}ms`);

    // Scroll performance should be consistently fast
    expect(avgScrollTime).toBeLessThan(500);
    
    // No individual scroll should be extremely slow
    scrollTimes.forEach((time, index) => {
      expect(time).toBeLessThan(1000);
    });
  });
});