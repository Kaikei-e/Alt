import { test, expect } from '@playwright/test';

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe('ActivityFeed Component - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a test page that renders the ActivityFeed component
    await page.goto('/test/activity-feed');
  });

  test('should render with glass effect and header (PROTECTED)', async ({ page }) => {
    const activityFeed = page.locator('[data-testid="activity-feed"]');

    await expect(activityFeed).toBeVisible();
    
    // Verify glassmorphism visual properties
    const styles = await activityFeed.evaluate(el => getComputedStyle(el));
    expect(styles.backdropFilter).toContain('blur');
    expect(styles.border).toContain('1px');
    
    // Check header section
    const header = activityFeed.locator('[data-testid="activity-header"]');
    await expect(header).toBeVisible();
    await expect(header).toHaveText('Recent Activity');
  });

  test('should display all activity items (PROTECTED)', async ({ page }) => {
    const activityFeed = page.locator('[data-testid="activity-feed"]');
    
    // Check all activity items are present
    const activityItems = activityFeed.locator('[data-testid="activity-item"]');
    await expect(activityItems).toHaveCount(4);
    
    // Check first activity item content
    const firstItem = activityItems.first();
    await expect(firstItem).toContainText('TechCrunch added');
    await expect(firstItem).toContainText('2 min ago');
  });

  test('should display empty state when no activities (PROTECTED)', async ({ page }) => {
    await page.goto('/test/activity-feed?empty=true');
    
    const emptyState = page.locator('[data-testid="empty-state"]');
    await expect(emptyState).toBeVisible();
    await expect(emptyState).toHaveText('No recent activity');
  });
});