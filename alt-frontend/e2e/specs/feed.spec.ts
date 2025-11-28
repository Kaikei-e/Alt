import { test, expect } from '@playwright/test';

test.describe('Feed Page', () => {
  test.beforeEach(async ({ page }) => {
    // Default mock for feed using CursorApi endpoint
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [
            {
              title: 'Test Article 1',
              description: 'Summary for article 1',
              link: 'https://example.com/1',
              published: new Date().toISOString(),
              author: { name: 'TechCrunch' },
            },
            {
              title: 'Test Article 2',
              description: 'Summary for article 2',
              link: 'https://example.com/2',
              published: new Date().toISOString(),
              author: { name: 'Zenn' },
            },
          ],
          next_cursor: 'next-cursor-id',
          has_more: true,
        }),
      });
    });
  });

  test('should display header elements', async ({ page, isMobile }) => {
    await page.goto('/desktop/feeds');

    // Check for Dashboard Header
    await expect(page.getByText('Alt Dashboard')).toBeVisible();

    // Check for Sidebar Navigation (active state)
    // "Feeds" should be active or visible
    if (!isMobile) {
      await expect(page.getByTestId('desktop-nav-link-feeds')).toBeVisible();
    }
  });

  test('should load and display feed items', async ({ page }) => {
    await page.goto('/desktop/feeds');

    // Verify feed items are displayed
    await expect(page.getByText('Test Article 1')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('Test Article 2')).toBeVisible({ timeout: 10000 });
    // Description might be truncated or not shown depending on view, checking title is safest
  });

  test('should display empty state when no articles', async ({ page }) => {
    // Override the route for this specific test
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [],
          next_cursor: null,
          has_more: false,
        }),
      });
    });

    await page.goto('/desktop/feeds');

    // Verify empty state message
    await expect(page.getByTestId('empty-state')).toBeVisible();
    await expect(page.getByText('No feeds available')).toBeVisible();
  });
});
