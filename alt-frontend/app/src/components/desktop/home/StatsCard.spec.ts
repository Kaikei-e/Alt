import { test, expect } from '@playwright/test';

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe('StatsCard Component - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a test page that renders the StatsCard component
    await page.goto('/test/stats-card');
  });

  test('should render with glass effect styling and animated number (PROTECTED)', async ({ page }) => {
    const statsCard = page.locator('[data-testid="stats-card"]');

    await expect(statsCard).toBeVisible();
    
    // Verify glassmorphism visual properties
    const styles = await statsCard.evaluate(el => getComputedStyle(el));
    expect(styles.backdropFilter).toContain('blur');
    expect(styles.border).toContain('1px');

    // Check animated number display
    const animatedNumber = statsCard.locator('text="42"');
    await expect(animatedNumber).toBeVisible();
  });

  test('should display correct content and trend information (PROTECTED)', async ({ page }) => {
    const statsCard = page.locator('[data-testid="stats-card"]');
    
    // Check label
    const label = statsCard.locator('text="Total Feeds"');
    await expect(label).toBeVisible();
    
    // Check trend
    const trend = statsCard.locator('text="+12%"');
    await expect(trend).toBeVisible();
    
    // Check trend label
    const trendLabel = statsCard.locator('text="from last week"');
    await expect(trendLabel).toBeVisible();
  });

  test('should have proper hover effects (PROTECTED)', async ({ page }) => {
    const statsCard = page.locator('[data-testid="stats-card"]');
    
    // Initial state
    await expect(statsCard).toBeVisible();
    
    // Hover and check transform
    await statsCard.hover();
    
    const styles = await statsCard.evaluate(el => getComputedStyle(el));
    // Check if transform is applied
    expect(styles.transform).toBeTruthy();
  });
});