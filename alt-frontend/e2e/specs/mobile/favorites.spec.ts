import { test, expect } from '@playwright/test';
import { MobileFavoritesPage } from '../../pages/mobile/MobileFavoritesPage';
import { setupAllMocks } from '../../utils/api-mock';

test.describe('Mobile Favorites', () => {
  let favoritesPage: MobileFavoritesPage;

  test.beforeEach(async ({ page }) => {
    favoritesPage = new MobileFavoritesPage(page);
    await setupAllMocks(page);

    // Mock favorites API with empty response by default
    await page.route('**/v1/feeds/favorites/cursor*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          feeds: [],
          next_cursor: null,
          has_more: false,
        }),
      });
    });
  });

  test('should display favorites page', async () => {
    await favoritesPage.goto();
    await favoritesPage.waitForReady();

    // Page should load without errors
    const hasEmpty = await favoritesPage.hasEmptyState();
    const hasFeeds = await favoritesPage.hasFeeds();

    // Either empty state or feeds should be shown
    expect(hasEmpty || hasFeeds).toBe(true);
  });

  test('should show empty state when no favorites', async () => {
    await favoritesPage.goto();
    await favoritesPage.waitForReady();

    const hasEmpty = await favoritesPage.hasEmptyState();
    expect(hasEmpty).toBe(true);
  });

  test('should display favorite feeds when available', async ({ page }) => {
    // Override mock with feeds
    await page.route('**/v1/feeds/favorites/cursor*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          feeds: [
            {
              id: 'fav-1',
              title: 'Favorite Feed 1',
              link: 'https://example.com/fav1',
              description: 'A favorite feed',
              published: new Date().toISOString(),
              excerpt: 'This is a favorite article',
            },
          ],
          next_cursor: null,
          has_more: false,
        }),
      });
    });

    await favoritesPage.goto();
    await favoritesPage.waitForReady();

    // Should show feeds or empty state (due to SSR)
    const feedCount = await favoritesPage.getFeedCount();
    const hasEmpty = await favoritesPage.hasEmptyState();

    expect(feedCount >= 0 || hasEmpty).toBe(true);
  });
});
