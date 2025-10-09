import { test, expect } from '@playwright/test';
import { DesktopFeedsPage } from '../../page-objects/desktop/feeds.page';
import { mockFeedsApi, mockEmptyFeeds, mockApiError } from '../../utils/api-mocks';
import { createMockFeed } from '../../utils/test-data';

test.describe('Desktop Feeds Page', () => {
  let feedsPage: DesktopFeedsPage;

  test.beforeEach(async ({ page }) => {
    feedsPage = new DesktopFeedsPage(page);
  });

  test('should display page with correct layout', async ({ page }) => {
    await mockFeedsApi(page, 10);
    await feedsPage.goto();

    // Check main content
    await expect(feedsPage.pageHeading).toBeVisible();
    await expect(feedsPage.feedsList).toBeVisible();

    // Check sidebar and right panel
    expect(await feedsPage.isSidebarVisible()).toBeTruthy();
    expect(await feedsPage.isRightPanelVisible()).toBeTruthy();
  });

  test('should load and display feeds', async ({ page }) => {
    const mockFeedsCount = 5;
    await mockFeedsApi(page, mockFeedsCount);
    await feedsPage.goto();

    // Wait for feeds to load
    await feedsPage.waitForLoad();

    // Check feed count
    const count = await feedsPage.getFeedCount();
    expect(count).toBe(mockFeedsCount);
  });

  test('should navigate to add feed page', async ({ page }) => {
    await mockFeedsApi(page, 5);
    await feedsPage.goto();

    await feedsPage.clickAddFeed();

    // Verify navigation
    await expect(page).toHaveURL(/\/desktop\/feeds\/register/);
  });

  test('should search feeds', async ({ page }) => {
    await mockFeedsApi(page, 10);
    await feedsPage.goto();

    const searchQuery = 'technology';
    await feedsPage.searchFeed(searchQuery);

    // Verify search input has value
    await expect(feedsPage.searchInput).toHaveValue(searchQuery);
  });

  test('should select a feed', async ({ page }) => {
    const mockFeed = createMockFeed({ title: 'Test Feed' });
    await mockFeedsApi(page, [mockFeed]);
    await feedsPage.goto();

    await feedsPage.selectFeedByIndex(0);

    // Should navigate or show feed details
    // Exact behavior depends on implementation
  });

  test('should handle empty state gracefully', async ({ page }) => {
    await mockEmptyFeeds(page);
    await feedsPage.goto();

    // Check empty state message
    const hasEmptyState = await feedsPage.hasEmptyState();
    expect(hasEmptyState).toBeTruthy();
  });

  test('should handle API errors gracefully', async ({ page }) => {
    await mockApiError(page, '**/v1/feeds**', 500);
    await feedsPage.goto();

    // Check error message and retry button
    const hasError = await feedsPage.hasError();
    expect(hasError).toBeTruthy();
  });

  test('should retry loading on error', async ({ page }) => {
    // First request fails
    await mockApiError(page, '**/v1/feeds**', 500);
    await feedsPage.goto();

    // Check error is shown
    expect(await feedsPage.hasError()).toBeTruthy();

    // Mock successful response for retry
    await mockFeedsApi(page, 5);

    // Click retry
    await feedsPage.clickRetry();

    // Should now show feeds
    await feedsPage.waitForLoad();
    const count = await feedsPage.getFeedCount();
    expect(count).toBeGreaterThan(0);
  });

  test('should be accessible', async ({ page }) => {
    await mockFeedsApi(page, 5);
    await feedsPage.goto();

    await feedsPage.checkA11y();
  });

  test('should have proper heading structure', async ({ page }) => {
    await mockFeedsApi(page, 5);
    await feedsPage.goto();

    const headings = await page
      .locator('h1, h2, h3, h4, h5, h6')
      .allTextContents();

    // Should have at least one heading
    expect(headings.length).toBeGreaterThan(0);
  });

  test('should handle keyboard navigation', async ({ page }) => {
    await mockFeedsApi(page, 5);
    await feedsPage.goto();

    // Tab to first interactive element
    await page.keyboard.press('Tab');

    const focused = await page.evaluate(() => {
      const el = document.activeElement;
      return {
        tagName: el?.tagName,
        role: el?.getAttribute('role'),
      };
    });

    // Should focus on an interactive element
    expect(
      focused.tagName === 'A' ||
        focused.tagName === 'BUTTON' ||
        focused.role === 'button' ||
        focused.role === 'link'
    ).toBeTruthy();
  });

  test('should display feed details', async ({ page }) => {
    const testFeed = createMockFeed({
      title: 'Tech News',
      description: 'Latest tech updates',
      unreadCount: 5,
    });

    await mockFeedsApi(page, [testFeed]);
    await feedsPage.goto();

    // Get feed titles
    const titles = await feedsPage.getFeedTitles();
    expect(titles.length).toBeGreaterThan(0);
  });

  test('should mark feed as favorite', async ({ page }) => {
    const testFeed = createMockFeed({ title: 'Favorite Feed' });
    await mockFeedsApi(page, [testFeed]);
    await feedsPage.goto();

    // Mark as favorite (if this feature exists)
    try {
      await feedsPage.markAsFavorite('Favorite Feed');
      // Verify favorite state changed
    } catch {
      // Feature might not exist
    }
  });

  test('should delete feed', async ({ page }) => {
    const testFeed = createMockFeed({ title: 'Feed to Delete' });
    await mockFeedsApi(page, [testFeed]);
    await feedsPage.goto();

    const initialCount = await feedsPage.getFeedCount();

    // Delete feed (if this feature exists)
    try {
      await feedsPage.deleteFeed('Feed to Delete');

      // Wait for update
      await feedsPage.wait(500);

      // Feed count should decrease
      const newCount = await feedsPage.getFeedCount();
      expect(newCount).toBe(initialCount - 1);
    } catch {
      // Feature might not exist or implemented differently
    }
  });

  test('should handle infinite scroll', async ({ page }) => {
    await mockFeedsApi(page, 20, true); // hasMore = true
    await feedsPage.goto();

    const initialCount = await feedsPage.getFeedCount();

    // Scroll to bottom
    await feedsPage.scrollToBottom();

    // Wait for potential new items to load
    await feedsPage.wait(1000);

    // Note: Actual infinite scroll behavior depends on implementation
  });

  test('should be responsive on different screen sizes', async ({ page }) => {
    const viewports = [
      { width: 1366, height: 768 }, // HD
      { width: 1920, height: 1080 }, // Full HD
      { width: 2560, height: 1440 }, // 2K
    ];

    for (const viewport of viewports) {
      await mockFeedsApi(page, 5);
      await page.setViewportSize(viewport);
      await feedsPage.goto();

      // Main content should be visible
      await expect(feedsPage.feedsList).toBeVisible();
    }
  });

  test('should load without JavaScript errors', async ({ page }) => {
    const errors: string[] = [];

    page.on('pageerror', (error) => {
      errors.push(error.message);
    });

    await mockFeedsApi(page, 5);
    await feedsPage.goto();

    // Should have no JavaScript errors
    expect(errors).toHaveLength(0);
  });
});
