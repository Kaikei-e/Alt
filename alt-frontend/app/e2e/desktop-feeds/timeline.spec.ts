import { test, expect } from '@playwright/test';

test.describe('DesktopTimeline Independent Scroll - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Mock feed data for testing
    await page.route('**/api/feeds*', async (route) => {
      const feeds = Array.from({ length: 20 }, (_, i) => ({
        id: `feed-${i}`,
        title: `Feed Title ${i}`,
        description: `Description for feed ${i}`,
        link: `https://example.com/feed-${i}`,
        published: new Date().toISOString(),
        isRead: i % 3 === 0,
        metadata: {
          source: { id: `source-${i}`, name: `Source ${i}` },
          tags: [`tag-${i}`],
          priority: 'medium'
        }
      }));
      
      await route.fulfill({
        json: { feeds, hasMore: true }
      });
    });

    await page.goto('/desktop/feeds');
  });

  test('should have independent scrollable container (PROTECTED)', async ({ page }) => {
    // Wait for timeline to load
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 5000 });
    
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();

    // Verify scroll container properties
    await expect(timeline).toHaveCSS('overflow-y', 'auto');
    await expect(timeline).toHaveCSS('overflow-x', 'hidden');
    
    // Verify max height is set (computed value should be less than viewport)
    const maxHeight = await timeline.evaluate(el => getComputedStyle(el).maxHeight);
    const maxHeightValue = parseFloat(maxHeight);
    expect(maxHeightValue).toBeGreaterThan(0);
    expect(maxHeightValue).toBeLessThan(720); // Default Playwright viewport height
  });

  test('should maintain scroll position and infinite scroll (PROTECTED)', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 5000 });
    
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    
    // First, ensure there's enough content to scroll
    await page.waitForSelector('[data-testid="desktop-timeline"] > div > div', { timeout: 5000 });
    
    // Scroll down within the timeline container
    await timeline.evaluate(el => {
      el.scrollTop = Math.min(300, el.scrollHeight - el.clientHeight);
    });
    
    // Wait for scroll to complete
    await page.waitForTimeout(100);
    
    // Verify scroll position is maintained
    const scrollTop = await timeline.evaluate(el => el.scrollTop);
    expect(scrollTop).toBeGreaterThan(0);
    
    // Verify infinite scroll trigger is present
    const loadMoreButton = page.locator('text=さらに読み込む');
    await expect(loadMoreButton).toBeVisible();
  });

  test('should be responsive across viewports (PROTECTED)', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 5000 });
    
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    
    // Test desktop viewport (lg)
    await page.setViewportSize({ width: 1024, height: 768 });
    await page.waitForTimeout(100);
    let maxHeight = await timeline.evaluate(el => getComputedStyle(el).maxHeight);
    let maxHeightValue = parseFloat(maxHeight);
    expect(maxHeightValue).toBeCloseTo(768 - 180, 0); // 588px
    
    // Test tablet viewport (md)
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.waitForTimeout(100);
    maxHeight = await timeline.evaluate(el => getComputedStyle(el).maxHeight);
    maxHeightValue = parseFloat(maxHeight);
    expect(maxHeightValue).toBeCloseTo(1024 - 140, 0); // 884px
    
    // Test mobile viewport (sm)
    await page.setViewportSize({ width: 375, height: 667 });
    await page.waitForTimeout(100);
    maxHeight = await timeline.evaluate(el => getComputedStyle(el).maxHeight);
    maxHeightValue = parseFloat(maxHeight);
    expect(maxHeightValue).toBeCloseTo(667, 0); // 100vh
  });
});