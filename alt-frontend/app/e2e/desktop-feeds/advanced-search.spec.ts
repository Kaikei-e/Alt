import { test, expect } from '@playwright/test';

test.describe('Advanced Search Functionality - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Mock feed data with diverse content for search testing
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = [
        {
          title: 'React 19 New Features Announced',
          description: 'React team announces new concurrent features and improved TypeScript support for better developer experience',
          link: 'https://example.com/react-19',
          published: new Date().toISOString(),
        },
        {
          title: 'Next.js Performance Optimization Guide',
          description: 'Complete guide to optimizing Next.js applications using Server Components and edge computing',
          link: 'https://example.com/nextjs-perf',
          published: new Date().toISOString(),
        },
        {
          title: 'TypeScript 5.0 Breaking Changes',
          description: 'Important breaking changes and migration guide for TypeScript developers',
          link: 'https://example.com/typescript-5',
          published: new Date().toISOString(),
        }
      ];

      await route.fulfill({
        json: {
          data: feeds,
          next_cursor: null
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);
  });

  test('should support multi-keyword search (PROTECTED)', async ({ page }) => {
    const searchInput = page.getByPlaceholder('Search feeds...');
    await expect(searchInput).toBeVisible();

    // Search for multiple keywords
    await searchInput.fill('React typescript');
    await page.keyboard.press('Enter');

    // Wait for search results
    await page.waitForTimeout(500);

    // Should find the React 19 post (contains both React and TypeScript)
    await expect(page.getByText('React 19 New Features Announced')).toBeVisible();

    // Should also find TypeScript 5.0 post (contains TypeScript)
    await expect(page.getByText('TypeScript 5.0 Breaking Changes')).toBeVisible();

    // Should not find Next.js post (doesn't contain both keywords)
    await expect(page.getByText('Next.js Performance Optimization Guide')).not.toBeVisible();
  });

  test('should highlight search results (PROTECTED)', async ({ page }) => {
    const searchInput = page.getByPlaceholder('Search feeds...');
    await searchInput.fill('React');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(500);

    // Check if search results are highlighted or indicated
    const searchResultsHeader = page.getByText('Search:', { exact: false });
    await expect(searchResultsHeader).toBeVisible();

    // Verify result count is displayed
    const resultCount = page.getByText('results', { exact: false });
    await expect(resultCount).toBeVisible();
  });

  test('should handle empty search gracefully (PROTECTED)', async ({ page }) => {
    const searchInput = page.getByPlaceholder('Search feeds...');

    // Clear search
    await searchInput.fill('');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(500);

    // Should show all feeds when search is empty
    await expect(page.getByText('React 19 New Features Announced')).toBeVisible();
    await expect(page.getByText('Next.js Performance Optimization Guide')).toBeVisible();
    await expect(page.getByText('TypeScript 5.0 Breaking Changes')).toBeVisible();
  });

  test('should handle no results found (PROTECTED)', async ({ page }) => {
    const searchInput = page.getByPlaceholder('Search feeds...');

    // Search for something that doesn't exist
    await searchInput.fill('nonexistent technology');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(500);

    // Should show no results message - try multiple possible messages
    const noResultsMessage1 = page.getByText('No results found');
    const noResultsMessage2 = page.getByText('No feeds found');
    const noResultsMessage3 = page.getByText('Nothing found');
    const noResultsMessage4 = page.getByText('No items found');

    const hasNoResultsMessage = await Promise.race([
      noResultsMessage1.isVisible(),
      noResultsMessage2.isVisible(),
      noResultsMessage3.isVisible(),
      noResultsMessage4.isVisible()
    ]);

    expect(hasNoResultsMessage).toBe(true);

    // Should not show any feed cards
    await expect(page.getByText('React 19 New Features Announced')).not.toBeVisible();
    await expect(page.getByText('Next.js Performance Optimization Guide')).not.toBeVisible();
  });
});