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
    // Best practice: set up wait promises BEFORE the action to avoid race conditions
    // WebKit (Safari) specific: bfcache (back/forward cache) may restore DOM but React
    // may not have re-rendered yet, so we need multiple wait strategies

    // Set up URL wait promise BEFORE goBack() action (Playwright best practice)
    const urlPromise = page.waitForURL(/\/desktop\/(feeds|home)/, {
      waitUntil: 'domcontentloaded',
      timeout: 10000
    });

    // Set up load state wait promises BEFORE goBack() action
    // WebKit needs both 'domcontentloaded' and 'load' states for bfcache restoration
    const domContentLoadedPromise = page.waitForLoadState('domcontentloaded', { timeout: 10000 });
    const loadStatePromise = page.waitForLoadState('load', { timeout: 10000 });

    // Perform the navigation action
    await page.goBack();

    // Wait for URL change (this ensures navigation started)
    await urlPromise;

    // Wait for DOM to be ready (domcontentloaded)
    await domContentLoadedPromise;

    // Wait for load state (WebKit bfcache restoration may need this)
    await loadStatePromise;

    // WebKit bfcache restoration: DOM exists but React may not have re-rendered yet
    // Use FeedPage's waitForFeeds() method for robust waiting
    // This uses expect().toBeVisible() which has built-in retry logic and handles
    // both cached page restoration and fresh page loads
    // The method automatically waits for feed cards to be visible and interactive
    await feedPage.waitForFeeds();

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

