import { test, expect } from '@playwright/test';

test.describe('Search Page', () => {
  test.beforeEach(async ({ page }) => {
    // Mock search results for "AI"
    await page.route('**/v1/articles/search?q=AI*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          {
            id: 's1',
            title: 'AI Revolution',
            content: 'How AI is changing the world.',
            url: 'https://example.com/ai',
            published_at: new Date().toISOString(),
          },
          {
            id: 's2',
            title: 'Future of AI',
            content: 'What comes next?',
            url: 'https://example.com/future-ai',
            published_at: new Date().toISOString(),
          },
        ]),
      });
    });

    // Mock empty results
    await page.route('**/v1/articles/search?q=Empty*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
    });
  });

  test('should display search results', async ({ page }) => {
    await page.goto('/desktop/articles/search');

    // Perform search
    await page.getByPlaceholder(/search/i).fill('AI');
    await page.getByRole('button', { name: /search/i }).click();

    // Verify results
    await expect(page.getByText('AI Revolution')).toBeVisible();
    await expect(page.getByText('Future of AI')).toBeVisible();
  });

  test('should display empty state', async ({ page }) => {
    await page.goto('/desktop/articles/search');

    // Perform search
    await page.getByPlaceholder(/search/i).fill('Empty');
    await page.getByRole('button', { name: /search/i }).click();

    // Verify empty message
    await expect(page.getByText(/no results/i)).toBeVisible({ timeout: 10000 });
  });
});
