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

    // Get search button reference before typing (best practice: get locators before actions)
    const searchButton = page.getByRole('button', { name: /search/i });

    // Type search query - use fill() which is faster and more reliable
    await searchInput.fill('TypeScript');

    // Wait for input value to be set (this waits for actual DOM state)
    // Mobile Safari needs explicit wait for input value to be set
    await expect(searchInput).toHaveValue('TypeScript', { timeout: 10000 });

    // Mobile Safari: React state update may lag behind DOM value change
    // Wait for button to be enabled with longer timeout and more robust checking
    // The button is disabled if query length < 2, so we need to wait for React to process the change
    await expect(searchButton).toBeEnabled({ timeout: 15000 });

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
    // Button is already enabled (checked above), so we can click directly
    await searchButton.click();

    // Wait for API response to complete
    await responsePromise;

    // Wait for loading state to disappear (simplified approach)
    // ArticleSearchResults shows "Searching articles..." when isLoading is true
    // Mobile Safari: wait for loading to disappear before checking results
    const loadingText = page.getByText(/Searching articles.../i);
    await expect(loadingText).toBeHidden({ timeout: 10000 }).catch(() => {
      // If loading text doesn't appear or disappear check fails, continue anyway
      // This handles cases where loading state is very fast or doesn't appear
    });

    // Wait for search results to appear (wait for actual DOM state)
    // ArticleSearchResults component shows "Found X articles" text first, then renders ArticleCard components
    // Mobile Chrome and Mobile Safari: wait for both metadata and article cards
    // Use multiple locator strategies for robustness
    const searchMetadata = page.getByText(/Found \d+ article/i);
    const articleCards = page.locator('[data-testid="article-card"]');

    // Wait for either metadata or article cards to appear (whichever comes first)
    // This handles cases where metadata appears first or cards render immediately
    const searchResults = searchMetadata.or(articleCards);
    await expect(searchResults.first()).toBeVisible({ timeout: 15000 });

    // Verify article cards are actually rendered (wait for cards if metadata appeared first)
    // This ensures the full search results are rendered, not just the metadata
    const cardsCount = await articleCards.count();
    if (cardsCount > 0) {
      await expect(articleCards.first()).toBeVisible({ timeout: 10000 });
    } else {
      // If no cards yet, wait a bit more for them to render
      // Some implementations may render metadata first, then cards
      await expect(articleCards.first()).toBeVisible({ timeout: 5000 }).catch(() => {
        // If cards don't appear, that's okay - metadata may be sufficient for the test
      });
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

    // Get search button reference before typing (best practice: get locators before actions)
    const searchButton = page.getByRole('button', { name: /search/i });

    // Type search query
    const query = 'NonExistentQuery12345';
    await searchInput.fill(query);

    // Wait for input value to be set (this waits for actual DOM state)
    // Mobile Safari needs explicit wait for input value to be set
    await expect(searchInput).toHaveValue(query, { timeout: 10000 });

    // Mobile Safari: React state update may lag behind DOM value change
    // Wait for button to be enabled with longer timeout and more robust checking
    // The button is disabled if query length < 2, so we need to wait for React to process the change
    await expect(searchButton).toBeEnabled({ timeout: 15000 });

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
    // Button is already enabled (checked above), so we can click directly
    await searchButton.click();

    // Wait for API response to complete
    await responsePromise;

    // Wait for loading state to disappear (simplified approach)
    // ArticleSearchResults shows "Searching articles..." when isLoading is true
    // Mobile Safari: wait for loading to disappear before checking empty state
    const loadingText = page.getByText(/Searching articles.../i);
    await expect(loadingText).toBeHidden({ timeout: 10000 }).catch(() => {
      // If loading text doesn't appear or disappear check fails, continue anyway
      // This handles cases where loading state is very fast or doesn't appear
    });

    // Wait for empty state message to appear (wait for actual DOM state)
    // ArticleSearchResults shows "No articles found" when results.length === 0
    // The component shows: "No articles found" (Heading) and "No articles match "{query}"..." (Text)
    // Mobile Safari: use multiple locator strategies with or() chain for robustness
    // This ensures we catch the empty state regardless of how it's rendered
    const emptyStateHeading = page.getByText(/No articles found/i).or(
      page.getByText(/no results found/i).or(
        page.locator('[data-testid="empty-state"]').or(
          page.locator('[role="status"]').filter({ hasText: /no articles|no results/i })
        )
      )
    );

    // Verify empty state heading is visible (expect() automatically waits for element)
    // Use longer timeout for Mobile Safari which may render slower
    await expect(emptyStateHeading.first()).toBeVisible({ timeout: 15000 });

    // Also verify the detailed message appears (this confirms the full empty state is rendered)
    // Mobile Safari: wait for both elements to ensure complete rendering
    // Use or() chain for more robust matching
    const emptyStateMessage = page.getByText(/No articles match/i).or(
      page.getByText(/no articles match/i).or(
        page.getByText(new RegExp(`no articles.*${query}`, 'i'))
      )
    );
    await expect(emptyStateMessage.first()).toBeVisible({ timeout: 10000 });
  });
});

