import { test, expect } from '@playwright/test';

test.describe('DesktopLayout Component', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the test page
    await page.goto('/test/desktop-layout');
    await page.waitForLoadState('domcontentloaded');
  });

  test('should render sidebar and main content areas', async ({ page }) => {
    const sidebar = page.locator('[data-testid="desktop-sidebar"]');
    const mainContent = page.locator('[data-testid="main-content"]');
    
    await expect(sidebar).toBeVisible();
    await expect(mainContent).toBeVisible();
  });

  test('should render ThemeToggle in the top right', async ({ page }) => {
    const themeToggle = page.locator('[data-testid="theme-toggle-button"]');
    await expect(themeToggle).toBeVisible();
  });

  test('should have proper layout structure', async ({ page }) => {
    const layoutContainer = page.locator('[data-testid="desktop-layout"]');
    await expect(layoutContainer).toBeVisible();
    
    // Check if sidebar has correct width
    const sidebar = page.locator('[data-testid="desktop-sidebar"]');
    const sidebarStyles = await sidebar.evaluate(el => getComputedStyle(el));
    expect(sidebarStyles.width).toBe('250px');
  });

  test('should display children content correctly', async ({ page }) => {
    await expect(page.getByText('Test Content')).toBeVisible();
  });
});