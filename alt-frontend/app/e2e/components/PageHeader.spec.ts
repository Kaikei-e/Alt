import { test, expect } from '@playwright/test';

test.describe('PageHeader Component', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the test page
    await page.goto('/test/page-header');
    await page.waitForLoadState('domcontentloaded');
  });

  test('should render title and description correctly', async ({ page }) => {
    await expect(page.getByText('Dashboard Overview')).toBeVisible();
    await expect(page.getByText('Monitor your RSS feeds and AI-powered content insights')).toBeVisible();
  });

  test('should apply correct styling to title and description', async ({ page }) => {
    const title = page.getByRole('heading', { name: 'Dashboard Overview' });
    const description = page.getByText('Monitor your RSS feeds and AI-powered content insights');

    // Check that elements have correct classes
    await expect(title).toHaveClass(/text-3xl font-bold/);
    await expect(description).toHaveClass(/text-lg text-gray-600/);
  });

  test('should have proper accessibility attributes', async ({ page }) => {
    const banner = page.getByRole('banner');
    const title = page.getByRole('heading', { name: 'Dashboard Overview' });
    
    await expect(banner).toBeVisible();
    await expect(title).toBeVisible();
  });
});