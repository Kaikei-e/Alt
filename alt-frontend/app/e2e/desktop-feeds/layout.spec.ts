import { test, expect } from '@playwright/test';

test.describe('Desktop Feeds Layout', () => {
  test('should display 3-column layout on desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1400, height: 900 });
    await page.goto('/desktop/feeds');

    // ヘッダーが表示される
    await expect(page.getByText('📰 Alt Feeds')).toBeVisible();
    
    // サイドバーが表示される
    await expect(page.getByText('Filters')).toBeVisible();
    
    // タイムラインが表示される（プレースホルダーメッセージ）
    await expect(page.getByText('フィードカードはTASK2で実装されます')).toBeVisible();
  });

  test('should adapt to mobile view', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto('/desktop/feeds');

    // モバイルではサイドバーが非表示
    await expect(page.getByText('Filters')).not.toBeVisible();
  });

  test('should have glassmorphism effects', async ({ page }) => {
    await page.goto('/desktop/feeds');

    const glassElements = page.locator('.glass');
    const count = await glassElements.count();
    
    expect(count).toBeGreaterThan(0);

    // CSS変数が適用されているか確認
    const styles = await glassElements.first().evaluate(el => getComputedStyle(el));
    expect(styles.backdropFilter).toContain('blur');
  });
});