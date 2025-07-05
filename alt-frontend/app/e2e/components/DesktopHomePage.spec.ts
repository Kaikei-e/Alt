import { test, expect } from '@playwright/test';

test.describe('DesktopHomePage Component', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the test page
    await page.goto('/test/desktop-home-page-simple');
    await page.waitForLoadState('domcontentloaded');
  });

  test('should render all main sections', async ({ page }) => {
    // Check if PageHeader is present
    await expect(page.getByText('Dashboard Overview')).toBeVisible();
    
    // Check if StatsGrid is present
    const statsGrid = page.locator('[data-testid="stats-grid"]');
    await expect(statsGrid).toBeVisible();
    
    // Check if ActivityFeed is present  
    const activityFeed = page.locator('[data-testid="activity-feed"]');
    await expect(activityFeed).toBeVisible();
    
    // Check if QuickActionsPanel is present
    const quickActions = page.locator('[data-testid="quick-actions-panel"]');
    await expect(quickActions).toBeVisible();
    
    // Check if CallToActionBar is present
    await expect(page.getByText('Ready to explore?')).toBeVisible();
  });

  test('should have proper layout structure', async ({ page }) => {
    const container = page.locator('[data-testid="desktop-home-container"]');
    await expect(container).toBeVisible();
    
    // Check for VStack structure
    const sections = container.locator('> *');
    const count = await sections.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should render sidebar navigation', async ({ page }) => {
    const sidebar = page.locator('[data-testid="desktop-sidebar"]');
    await expect(sidebar).toBeVisible();
    
    // Check for navigation items in sidebar specifically
    await expect(sidebar.getByRole('link', { name: 'Dashboard' })).toBeVisible();
    await expect(sidebar.getByRole('link', { name: 'Feeds' })).toBeVisible();
  });
});