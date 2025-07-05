import { test, expect } from '@playwright/test';

test.describe('DesktopSidebar Component', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the test page
    await page.goto('/test/desktop-sidebar');
    await page.waitForLoadState('domcontentloaded');
  });

  test('should render logo and navigation items correctly', async ({ page }) => {
    // Check logo text and subtext
    await expect(page.getByText('Alt RSS')).toBeVisible();
    await expect(page.getByText('Feed Reader')).toBeVisible();
    
    // Check navigation items
    await expect(page.getByRole('link', { name: 'Dashboard' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Feeds' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Statistics' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Settings' })).toBeVisible();
  });

  test('should highlight active navigation item', async ({ page }) => {
    const activeItem = page.getByRole('link', { name: 'Dashboard' });
    await expect(activeItem).toHaveClass(/active/);
  });

  test('should have proper accessibility attributes', async ({ page }) => {
    const nav = page.getByRole('navigation');
    await expect(nav).toBeVisible();
    await expect(nav).toHaveAttribute('aria-label', 'Main navigation');
    
    // Check that navigation items are accessible
    const navItems = page.locator('nav a');
    const count = await navItems.count();
    expect(count).toBeGreaterThan(0);
  });
});