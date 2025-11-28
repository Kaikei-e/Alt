import { test, expect } from '@playwright/test';

test.describe('Article Page', () => {
  test.beforeEach(async ({ page }) => {
    // Default mock for a standard article
    await page.route('**/v1/articles/123', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: '123',
          title: 'Deep Dive into AI',
          content: '<p>This is a deep dive into Artificial Intelligence.</p>',
          url: 'https://example.com/123',
          published_at: new Date().toISOString(),
        }),
      });
    });
  });

  test('should display standard article details', async ({ page }) => {
    await page.goto('/desktop/articles/123');

    // Verify Title
    await expect(page.getByRole('heading', { name: 'Deep Dive into AI' })).toBeVisible();

    // Verify Content
    await expect(page.getByText('This is a deep dive into Artificial Intelligence.')).toBeVisible();

    // Author is not displayed in current implementation
  });

  test('should handle 404 error', async ({ page }) => {
    // Mock 404 response
    await page.route('**/v1/articles/999', async (route) => {
      await route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Article not found' }),
      });
    });

    await page.goto('/desktop/articles/999');

    // Verify error message
    await expect(page.getByText(/not found/i)).toBeVisible();
  });

  test('should handle 500 error', async ({ page }) => {
    // Mock 500 response
    await page.route('**/v1/articles/500', async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Internal Server Error' }),
      });
    });

    await page.goto('/desktop/articles/500');

    // Verify error message
    await expect(page.getByText(/error|wrong/i)).toBeVisible();
  });
});
