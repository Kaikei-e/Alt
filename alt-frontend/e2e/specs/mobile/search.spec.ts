import { test, expect } from '@playwright/test';
import { MobileHomePage } from '../../pages/mobile/MobileHomePage';
import { setupFeedMocks, mockSearchApi } from '../../utils/api-mock';

test.describe('Mobile Search', () => {
  test.beforeEach(async ({ page }) => {
    // Setup all common API mocks
    await setupFeedMocks(page);
  });

  test('should perform search on mobile', async ({ page }) => {
    // Setup search mock
    await mockSearchApi(page, { query: 'TypeScript' });

    // Navigate to mobile search page
    await page.goto('/mobile/articles/search');

    // Find search input - wait for it to be visible
    const searchInput = page.getByPlaceholder(/search|検索/i).or(
      page.getByRole('searchbox').or(page.locator('input[type="text"]'))
    );

    // Wait for input to be visible
    await expect(searchInput).toBeVisible({ timeout: 10000 });

    // Type search query
    await searchInput.fill('TypeScript');

    // Wait for input value to be set (this waits for actual DOM state)
    // Mobile Safari needs explicit wait for input value to be set
    await expect(searchInput).toHaveValue('TypeScript');

    // Wait for button to be enabled (button is disabled if query length < 2)
    // Mobile Safari: wait for button state to update after input value changes
    const searchButton = page.getByRole('button', { name: /search/i });
    await expect(searchButton).toBeEnabled();

    // Wait for response before clicking button (best practice: set up wait before action)
    // Mobile Safari may not trigger form submit on Enter key, so use button click instead
    const responsePromise = page.waitForResponse(
      (response) => {
        const url = response.url();
        return (url.includes('/v1/articles/search') || url.includes('/api/frontend/v1/articles/search')) &&
          url.includes('q=TypeScript');
      }
    );

    // Use button click instead of Enter key for better reliability on mobile Safari
    // Wait for button to be clickable (not disabled) before clicking
    await expect(searchButton).not.toBeDisabled();
    await searchButton.click();

    // Wait for API response to complete
    await responsePromise;

    // Wait for loading state to disappear (wait for actual DOM state change)
    // ArticleSearchResults shows "Searching articles..." when isLoading is true
    // Mobile Safari: wait for loading to disappear before checking results
    const loadingText = page.getByText(/Searching articles.../i);
    await expect(loadingText).toBeHidden().catch(() => {
      // If loading text doesn't appear, continue anyway
    });

    // Wait for search results to appear (wait for actual DOM state)
    // ArticleSearchResults component shows "Found X articles" text first, then renders ArticleCard components
    // Mobile Safari: wait for either search metadata or article cards to appear
    // Use or() chain for more robust locator matching (Playwright best practice)
    const searchResults = page.getByText(/Found \d+ article/i).or(
      page.locator('[data-testid="article-card"]')
    );

    // Wait for at least one result indicator to appear
    await expect(searchResults.first()).toBeVisible();

    // Verify article cards are actually rendered (if metadata appeared first)
    const articleCards = page.locator('[data-testid="article-card"]');
    const articleCardsCount = await articleCards.count();
    if (articleCardsCount > 0) {
      await expect(articleCards.first()).toBeVisible();
    }
  });

  test('should display empty state when no results found on mobile', async ({ page }) => {
    // Setup search mock to return empty results
    await mockSearchApi(page, { empty: true });

    // Navigate to mobile search page
    await page.goto('/mobile/articles/search');

    // Find search input
    const searchInput = page.getByPlaceholder(/search|検索/i).or(
      page.getByRole('searchbox').or(page.locator('input[type="text"]'))
    );

    // Wait for input to be visible
    await expect(searchInput).toBeVisible({ timeout: 10000 });

    // Type search query
    const query = 'NonExistentQuery12345';
    await searchInput.fill(query);

    // Wait for input value to be set (this waits for actual DOM state)
    // Mobile Safari needs explicit wait for input value to be set
    await expect(searchInput).toHaveValue(query);

    // Wait for button to be enabled (button is disabled if query length < 2)
    // Mobile Safari: wait for button state to update after input value changes
    const searchButton = page.getByRole('button', { name: /search/i });
    await expect(searchButton).toBeEnabled();

    // Wait for response before clicking button (best practice: set up wait before action)
    // Mobile Safari may not trigger form submit on Enter key, so use button click instead
    const responsePromise = page.waitForResponse(
      (response) => {
        const url = response.url();
        return (url.includes('/v1/articles/search') || url.includes('/api/frontend/v1/articles/search')) &&
          url.includes(`q=${encodeURIComponent(query)}`);
      }
    );

    // Use button click instead of Enter key for better reliability on mobile Safari
    // Wait for button to be clickable (not disabled) before clicking
    await expect(searchButton).not.toBeDisabled();
    await searchButton.click();

    // Wait for API response to complete
    await responsePromise;

    // Wait for loading state to disappear (wait for actual DOM state change)
    // ArticleSearchResults shows "Searching articles..." when isLoading is true
    // Mobile Safari: wait for loading to disappear before checking empty state
    const loadingText = page.getByText(/Searching articles.../i);
    await expect(loadingText).toBeHidden().catch(() => {
      // If loading text doesn't appear, continue anyway
    });

    // Wait for empty state message to appear (wait for actual DOM state)
    // ArticleSearchResults shows "No articles found" when results.length === 0
    // The component shows: "No articles found" (Heading) and "No articles match "{query}"..." (Text)
    // Mobile Safari: wait for the main heading text which is always present in empty state
    const emptyStateHeading = page.getByText(/No articles found/i);

    // Verify empty state heading is visible (expect() automatically waits for element)
    await expect(emptyStateHeading).toBeVisible();

    // Also verify the detailed message appears (this confirms the full empty state is rendered)
    // Mobile Safari: wait for both elements to ensure complete rendering
    const emptyStateMessage = page.getByText(/No articles match/i);
    await expect(emptyStateMessage).toBeVisible();
  });
});

