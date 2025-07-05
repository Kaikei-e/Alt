import { test, expect } from '@playwright/test';

test.describe('Desktop Feeds Performance', () => {
  test('should load initial page within performance budget', async ({ page }) => {
    // Start performance monitoring
    await page.goto('/desktop/feeds', { waitUntil: 'networkidle' });

    // Core Web Vitalsの測定
    const metrics = await page.evaluate(() => {
      return new Promise<{ fcp?: number; lcp?: number }>((resolve) => {
        new PerformanceObserver((list) => {
          const entries = list.getEntries();
          const vitals: { fcp?: number; lcp?: number } = {};

          entries.forEach((entry) => {
            if (entry.name === 'first-contentful-paint') {
              vitals.fcp = entry.startTime;
            }
            if (entry.name === 'largest-contentful-paint') {
              vitals.lcp = entry.startTime;
            }
          });

          resolve(vitals);
        }).observe({ entryTypes: ['paint', 'largest-contentful-paint'] });
      });
    });

    // パフォーマンス要件の確認
    if (metrics.fcp) expect(metrics.fcp).toBeLessThan(1500); // FCP < 1.5s
    if (metrics.lcp) expect(metrics.lcp).toBeLessThan(2500); // LCP < 2.5s

    // Chakra UIのテーマが正しく適用されているか確認
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    await expect(feedCard).toBeVisible();

    const styles = await feedCard.evaluate(el => getComputedStyle(el));
    expect(styles.borderRadius).toBe('var(--radius-xl)');
  });

  test('should handle large number of feeds efficiently', async ({ page }) => {
    await page.goto('/desktop/feeds');

    // 大量データセットのシミュレーション
    await page.evaluate(() => {
      (window as any).mockLargeDataset = true;
    });

    await page.reload();

    // 応答性の確認
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    await expect(feedCard).toBeVisible({ timeout: 5000 });

    // インタラクション応答性
    const interactionStart = Date.now();
    await feedCard.click();
    const interactionEnd = Date.now();

    expect(interactionEnd - interactionStart).toBeLessThan(100);
  });

  test('should maintain performance during filtering', async ({ page }) => {
    await page.goto('/desktop/feeds');

    // 複数フィルターの高速適用
    const filterStart = Date.now();

    await page.click('text=Unread');
    await page.click('text=TechCrunch');
    await page.fill('input[placeholder="Search feeds..."]', 'AI');

    // フィルター適用の待機
    await page.waitForTimeout(500);

    const filterEnd = Date.now();
    expect(filterEnd - filterStart).toBeLessThan(1000);

    // 結果が表示されるか確認
    const filteredFeeds = page.locator('[data-testid^="desktop-feed-card-"]');
    await expect(filteredFeeds.first()).toBeVisible();

    // CSS変数が正しく適用されているか確認
    const styles = await filteredFeeds.first().evaluate(el => getComputedStyle(el));
    expect(styles.background).toContain('var(');
  });
});