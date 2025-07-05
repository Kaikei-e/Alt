import { test, expect } from '@playwright/test';

test.describe('PageHeader Component', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the test page with longer timeout
    await page.goto('/test/page-header', { waitUntil: 'networkidle', timeout: 60000 });

    // Wait for the PageHeader component to be visible
    await expect(page.getByRole('banner')).toBeVisible({ timeout: 30000 });
  });

  test('should render title and description correctly', async ({ page }) => {
    await expect(page.getByText('Dashboard Overview')).toBeVisible();
    await expect(page.getByText('Monitor your RSS feeds and AI-powered content insights')).toBeVisible();
  });

  test('should apply correct styling to title and description', async ({ page }) => {
    const title = page.getByRole('heading', { name: 'Dashboard Overview' });
    const description = page.getByText('Monitor your RSS feeds and AI-powered content insights');

    // Check that elements are visible first
    await expect(title).toBeVisible();
    await expect(description).toBeVisible();

    // Check that elements have correct classes
    await expect(title).toHaveClass(/text-3xl/);
    await expect(title).toHaveClass(/font-bold/);
    await expect(description).toHaveClass(/text-lg/);
    await expect(description).toHaveClass(/text-gray-600/);
  });

  test('should have proper accessibility attributes', async ({ page }) => {
    const banner = page.getByRole('banner');
    const title = page.getByRole('heading', { name: 'Dashboard Overview' });

    await expect(banner).toBeVisible();
    await expect(title).toBeVisible();

    // Check that title is properly structured
    await expect(title).toHaveAttribute('class');
  });
});