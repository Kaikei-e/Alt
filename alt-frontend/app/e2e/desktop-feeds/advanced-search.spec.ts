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
    await searchInput.fill('nonexistent technology xyz123');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(1000); // Increased wait time

    // Should show no results message - comprehensive checking
    const noResultsSelectors = [
      page.getByText('No results found'),
      page.getByText('No feeds found'),
      page.getByText('Nothing found'),
      page.getByText('No items found'),
      page.getByText('No search results'),
      page.getByText(/no.*results/i),
      page.getByText(/no.*found/i),
      page.getByText(/nothing.*found/i),
      page.locator('[data-testid="no-results"]'),
      page.locator('[data-testid="empty-state"]'),
    ];

    let noResultsFound = false;
    for (const selector of noResultsSelectors) {
      const isVisible = await selector.isVisible().catch(() => false);
      if (isVisible) {
        noResultsFound = true;
        break;
      }
    }

    if (noResultsFound) {
      expect(noResultsFound).toBe(true);
    } else {
      // Alternative: Check that no feed cards are visible
      const feedCards = page.locator('[data-testid^="feed-item-"]');
      const feedCount = await feedCards.count();

      // If no specific "no results" message, at least verify no feeds are shown
      expect(feedCount).toBe(0);

      // Verify search interface is still functional
      await expect(searchInput).toBeVisible();
      const searchValue = await searchInput.inputValue();
      expect(searchValue).toBe('nonexistent technology xyz123');

      console.log('No specific "no results" message found, but search correctly filtered content');
    }

    // Should not show any of the original feed cards
    const originalFeeds = [
      page.getByText('React 19 New Features Announced'),
      page.getByText('Next.js Performance Optimization Guide'),
      page.getByText('TypeScript 5.0 Breaking Changes'),
    ];

    for (const feed of originalFeeds) {
      const isVisible = await feed.isVisible().catch(() => false);
      expect(isVisible).toBe(false);
    }
  });
});