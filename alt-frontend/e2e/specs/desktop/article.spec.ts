import { test, expect } from '@playwright/test';
import { FeedPage } from '../../pages/desktop/FeedPage';
import { ArticlePage } from '../../pages/desktop/ArticlePage';
import { setupFeedMocks } from '../../utils/api-mock';
import { assertArticleDetail, assertToastNotification } from '../../utils/assertions';

test.describe('Desktop Article', () => {
  test.beforeEach(async ({ page }) => {
    // Setup all common API mocks
    await setupFeedMocks(page);
  });

  test('should navigate to article detail page and display content', async ({ page }) => {
    const articlePage = new ArticlePage(page);

    // Navigate directly to article detail page (since clicking feed card opens external link)
    await articlePage.goto('feed-1');

    // Wait for article page to load
    await articlePage.waitForArticle();

    // Verify URL changed to article detail page
    const currentUrl = articlePage.getUrl();
    expect(currentUrl).toMatch(/\/desktop\/articles\/feed-1/);

    // Verify article title is displayed
    const articleTitle = await articlePage.getTitle();
    expect(articleTitle.length).toBeGreaterThan(0);
    expect(articleTitle).toContain('React 19');

    // Verify article body is displayed
    await assertArticleDetail(articlePage.articleTitle, articlePage.articleBody);
  });

  test('should navigate back to feed list', async ({ page }) => {
    const feedPage = new FeedPage(page);
    const articlePage = new ArticlePage(page);

    // Navigate to feeds page
    await feedPage.goto();
    await feedPage.waitForFeeds();

    // Get the initial URL
    const initialUrl = feedPage.getUrl();

    // Navigate directly to article detail page
    await articlePage.goto('feed-1');
    await articlePage.waitForArticle();

    // Verify we're on article page
    const articleUrl = articlePage.getUrl();
    expect(articleUrl).not.toBe(initialUrl);
    expect(articleUrl).toMatch(/\/desktop\/articles\/feed-1/);

    // Article detail page doesn't have a back button, so use browser back
    // Best practice: wait for URL change and feeds to be loaded after navigation
    // Set up wait before action to avoid race conditions
    const urlPromise = page.waitForURL(/\/desktop\/(feeds|home)/, {
      waitUntil: 'domcontentloaded'
    });
    await page.goBack();
    await urlPromise;

    // Wait for page to be fully loaded (wait for actual page state)
    await page.waitForLoadState('domcontentloaded');

    // Wait for feeds to be actually loaded and visible (wait for real DOM state)
    // Browser back navigation may restore from cache, so we wait for DOM elements
    // rather than API responses which may not fire again
    // First check if feed cards are already visible (cached page)
    const feedCards = page.locator('[data-testid^="desktop-feed-card-"]');
    const feedCardsCount = await feedCards.count();

    if (feedCardsCount === 0) {
      // If no feed cards visible, wait for them to appear
      // This handles the case where page is restored from cache but React hasn't rendered yet
      await feedPage.waitForFeeds();
    } else {
      // If feed cards are already visible, verify at least one is visible
      await expect(feedCards.first()).toBeVisible();
    }

    // Verify we're back on the feed list
    const finalUrl = page.url();
    expect(finalUrl).toMatch(/\/desktop\/(feeds|home)/);
  });

  test.skip('should bookmark an article', async ({ page }) => {
    // Skip this test as bookmark button is not yet implemented in article detail page
    // TODO: Re-enable when bookmark functionality is added to /desktop/articles/[id] page
    const articlePage = new ArticlePage(page);

    // Navigate directly to article detail page
    await articlePage.goto('feed-1');
    await articlePage.waitForArticle();

    // Intercept bookmark API call
    let bookmarkApiCalled = false;
    page.on('request', (request) => {
      if (
        (request.url().includes('/api/frontend/bookmarks') ||
          request.url().includes('/api/frontend/v1/bookmarks')) &&
        request.method() === 'POST'
      ) {
        bookmarkApiCalled = true;
      }
    });

    // Click bookmark button (if it exists)
    await articlePage.clickBookmark();
    // Wait for toast notification
    await assertToastNotification(articlePage.toastNotification, /saved|保存/i);
    // Verify bookmark API was called
    expect(bookmarkApiCalled).toBeTruthy();
  });
});

