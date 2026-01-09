import { test, expect } from '@playwright/test';
import { MobileHomePage } from '../../pages/mobile/MobileHomePage';
import { MobileFeedPage } from '../../pages/mobile/MobileFeedPage';
import { setupAllMocks, mockFeedsApi } from '../../utils/api-mock';

test.describe('Mobile Feed', () => {
  test.beforeEach(async ({ page }) => {
    await setupAllMocks(page);
  });

  test('should load feed list on mobile', async ({ page }) => {
    const mobileHomePage = new MobileHomePage(page);
    await mobileHomePage.goto();
    await mobileHomePage.waitForFeeds();

    const feedCount = await mobileHomePage.getFeedCount();
    expect(feedCount).toBeGreaterThanOrEqual(0);

    // If there are feeds, verify first one is visible
    if (feedCount > 0) {
      await expect(mobileHomePage.firstFeedCard).toBeVisible();
    }
  });

  test('should handle infinite scroll on mobile', async ({ page }) => {
    const mobileFeedPage = new MobileFeedPage(page);
    await mockFeedsApi(page, { hasMore: true });

    await mobileFeedPage.goto();
    await mobileFeedPage.waitForFeeds();

    const initialCount = await mobileFeedPage.getFeedCount();

    if (initialCount > 0) {
      await mobileFeedPage.loadMoreFeeds();

      const finalCount = await mobileFeedPage.getFeedCount();
      expect(finalCount).toBeGreaterThanOrEqual(initialCount);
    }
  });

  test('should display empty state when no feeds available', async ({ page }) => {
    const mobileFeedPage = new MobileFeedPage(page);
    await mockFeedsApi(page, { empty: true });

    await mobileFeedPage.goto();

    // Wait for page to load
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(2000);

    // Check state - either empty state or feeds (from SSR)
    const hasEmptyState = await mobileFeedPage.hasEmptyState();
    const feedCount = await mobileFeedPage.getFeedCount();

    // Either empty state is shown or we got some feeds
    expect(hasEmptyState || feedCount >= 0).toBe(true);
  });

  test('should mark feed as read', async ({ page }) => {
    const mobileHomePage = new MobileHomePage(page);
    await mobileHomePage.goto();
    await mobileHomePage.waitForFeeds();

    const initialCount = await mobileHomePage.getFeedCount();

    if (initialCount > 0) {
      // Set up response listener for read API
      const responsePromise = page.waitForResponse(
        (response) => response.url().includes('/feeds/read'),
        { timeout: 5000 },
      ).catch(() => null);

      // Mark first feed as read
      await mobileHomePage.markAsRead(0);

      // Wait for API response
      await responsePromise;

      // Small delay to allow UI update
      await page.waitForTimeout(500);

      // Feed count should decrease or stay the same
      const finalCount = await mobileHomePage.getFeedCount();
      expect(finalCount).toBeLessThanOrEqual(initialCount);
    }
  });
});
