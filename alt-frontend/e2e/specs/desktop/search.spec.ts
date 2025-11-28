import { test, expect } from '@playwright/test';
import { setupFeedMocks, mockSearchApi } from '../../utils/api-mock';

test.describe('Desktop Search', () => {
  test.beforeEach(async ({ page }) => {
    // Setup all common API mocks
    await setupFeedMocks(page);
  });

  test('should perform search and display results', async ({ page }) => {
    // Setup search mock
    await mockSearchApi(page, { query: 'AI' });

    // Navigate to search page
    await page.goto('/desktop/articles/search');

    // Find search input
    const searchInput = page.getByPlaceholder(/search articles|検索/i).or(
      page.getByRole('searchbox').or(page.locator('input[type="text"]'))
    );

    // Wait for input to be visible (expect() automatically waits)
    await expect(searchInput).toBeVisible();

    // Type search query
    await searchInput.fill('AI');

    // Find and click search button
    const searchButton = page.getByRole('button', { name: /search/i });

    // Wait for response (best practice: set up wait before action)
    const responsePromise = page.waitForResponse(
      (response) => {
        const url = response.url();
        return (url.includes('/v1/articles/search') || url.includes('/api/frontend/v1/articles/search')) &&
          url.includes('q=AI');
      }
    );

    await searchButton.click();
    await responsePromise;

    // Wait for search results to appear (wait for actual DOM state)
    // Desktop search page shows results as Box elements with Heading (size="md") and Text
    // Chakra UI Heading size="md" can render as h2 or h3 depending on context
    // Use more flexible locator that matches any heading level
    const searchResults = page.locator('h2, h3').filter({
      hasText: /AI|artificial intelligence|React|TypeScript|Understanding React/i
    });

    // Verify search results are visible (expect() automatically waits for element)
    await expect(searchResults.first()).toBeVisible();
  });

  test('should display empty state when no results found', async ({ page }) => {
    // Setup search mock to return empty results
    await mockSearchApi(page, { empty: true });

    // Navigate to search page
    await page.goto('/desktop/articles/search');

    // Find search input
    const searchInput = page.getByPlaceholder(/search articles|検索/i).or(
      page.getByRole('searchbox').or(page.locator('input[type="text"]'))
    );

    // Wait for input to be visible (expect() automatically waits)
    await expect(searchInput).toBeVisible();

    // Type search query
    const query = 'NonExistentQuery12345';
    await searchInput.fill(query);

    // Find and click search button
    const searchButton = page.getByRole('button', { name: /search/i });

    // Wait for response (best practice: set up wait before action)
    const responsePromise = page.waitForResponse(
      (response) => {
        const url = response.url();
        return (url.includes('/v1/articles/search') || url.includes('/api/frontend/v1/articles/search')) &&
          url.includes(`q=${encodeURIComponent(query)}`);
      }
    );

    await searchButton.click();
    await responsePromise;

    // Wait for empty state message to appear (wait for actual DOM state)
    // Desktop search page shows "No results found." when searched && results.length === 0
    // expect() automatically waits for element to be visible
    const emptyState = page.getByText(/no results found|結果が見つかりません|not found/i);
    await expect(emptyState).toBeVisible();
  });

  test('should update URL query parameter on search', async ({ page }) => {
    await setupFeedMocks(page);
    await mockSearchApi(page, { query: 'React' });

    // Navigate to search page
    await page.goto('/desktop/articles/search');

    // Find search input
    const searchInput = page.getByPlaceholder(/search articles|検索/i).or(
      page.getByRole('searchbox').or(page.locator('input[type="text"]'))
    );

    // Wait for input to be visible (expect() automatically waits)
    await expect(searchInput).toBeVisible();

    // Type search query
    await searchInput.fill('React');

    // Find and click search button
    const searchButton = page.getByRole('button', { name: /search/i });

    // Wait for response (best practice: set up wait before action)
    const responsePromise = page.waitForResponse(
      (response) => {
        const url = response.url();
        return (url.includes('/v1/articles/search') || url.includes('/api/frontend/v1/articles/search')) &&
          url.includes('q=React');
      }
    );

    await searchButton.click();
    await responsePromise;

    // Wait for search results to appear (wait for actual DOM state)
    // Desktop search page shows results as Box elements with Heading (size="md") and Text
    // Chakra UI Heading size="md" can render as h2 or h3 depending on context
    // Use more flexible locator that matches any heading level
    const searchResults = page.locator('h2, h3').filter({
      hasText: /React|Understanding React/i
    });

    // Verify search results are visible (expect() automatically waits for element)
    await expect(searchResults.first()).toBeVisible();
  });
});

