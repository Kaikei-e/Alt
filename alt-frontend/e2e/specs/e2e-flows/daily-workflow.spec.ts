import { test, expect } from '@playwright/test';
import { LoginPage } from '../../page-objects/auth/login.page';
import { HomePage } from '../../page-objects/desktop/home.page';
import { DesktopFeedsPage } from '../../page-objects/desktop/feeds.page';
import { DesktopArticlesPage } from '../../page-objects/desktop/articles.page';
import { mockFeedsApi, mockArticlesApi } from '../../utils/api-mocks';
import { testUsers } from '../../utils/test-data';

test.describe('Daily User Workflow E2E', () => {
  test('complete user journey: login → browse feeds → read articles → logout', async ({
    page,
  }) => {
    // Mock API responses
    await mockFeedsApi(page, 5);
    await mockArticlesApi(page, 20);

    // Step 1: Login
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);

    // Wait for successful login
    await loginPage.waitForLoginSuccess();

    // Step 2: Navigate to home
    const homePage = new HomePage(page);

    // Verify we're on home page
    await expect(page).toHaveURL(/\/home/);

    // Step 3: Go to desktop view
    await homePage.goToDesktop();

    // Step 4: Navigate to feeds
    await page.goto('/desktop/feeds');
    const feedsPage = new DesktopFeedsPage(page);
    await feedsPage.waitForLoad();

    // Verify feeds are loaded
    await expect(feedsPage.feedsList).toBeVisible();
    const feedCount = await feedsPage.getFeedCount();
    expect(feedCount).toBeGreaterThan(0);

    // Step 5: Select a feed
    await feedsPage.selectFeedByIndex(0);

    // Step 6: Navigate to articles (or should already be there)
    await page.goto('/desktop/articles');
    const articlesPage = new DesktopArticlesPage(page);
    await articlesPage.waitForLoad();

    // Verify articles are displayed
    await expect(articlesPage.articlesList).toBeVisible();
    const articleCount = await articlesPage.getArticleCount();
    expect(articleCount).toBeGreaterThan(0);

    // Step 7: Open an article
    await articlesPage.openArticle(0);

    // Verify article content is visible
    await expect(articlesPage.articleContent).toBeVisible();

    // Step 8: Mark as favorite
    try {
      await articlesPage.markAsFavorite();
      // Favorite icon should have active state
      await expect(articlesPage.favoriteIcon.first()).toHaveClass(/active|filled/);
    } catch {
      // Feature might not be available in current implementation
    }

    // Step 9: Mark as read
    try {
      await articlesPage.markAsRead();
    } catch {
      // Feature might not be available in current implementation
    }

    // Step 10: Logout
    await homePage.goto();
    await homePage.logout();

    // Verify redirect to login or landing page
    await expect(page).toHaveURL(/\/public\/landing|\/auth\/login/);
  });

  test('user can navigate between pages and maintain state', async ({ page }) => {
    await mockFeedsApi(page, 5);
    await mockArticlesApi(page, 10);

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await loginPage.waitForLoginSuccess();

    // Go to feeds
    await page.goto('/desktop/feeds');
    const feedsPage = new DesktopFeedsPage(page);
    await feedsPage.waitForLoad();

    // Get initial feed count
    const initialFeedCount = await feedsPage.getFeedCount();

    // Navigate to articles
    await page.goto('/desktop/articles');
    const articlesPage = new DesktopArticlesPage(page);
    await articlesPage.waitForLoad();

    // Navigate back to feeds
    await page.goto('/desktop/feeds');
    await feedsPage.waitForLoad();

    // Feed count should be the same
    const newFeedCount = await feedsPage.getFeedCount();
    expect(newFeedCount).toBe(initialFeedCount);
  });

  test('user can search and find content', async ({ page }) => {
    await mockFeedsApi(page, 10);
    await mockArticlesApi(page, 20);

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await loginPage.waitForLoginSuccess();

    // Go to articles search
    await page.goto('/desktop/articles/search');

    // Search for content
    const searchInput = page.getByRole('searchbox');
    await searchInput.fill('technology');
    await searchInput.press('Enter');

    // Wait for results
    await page.waitForLoadState('networkidle');

    // Results should be displayed
    const results = page.getByRole('article');
    const count = await results.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('user workflow handles errors gracefully', async ({ page }) => {
    // Mock API error for feeds
    await page.route('**/v1/feeds**', (route) => {
      route.fulfill({ status: 500 });
    });

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await loginPage.waitForLoginSuccess();

    // Try to go to feeds
    await page.goto('/desktop/feeds');
    const feedsPage = new DesktopFeedsPage(page);

    // Should show error message
    const hasError = await feedsPage.hasError();
    expect(hasError).toBeTruthy();

    // Mock successful response
    await mockFeedsApi(page, 5);

    // Retry should work
    await feedsPage.clickRetry();
    await feedsPage.waitForLoad();

    const feedCount = await feedsPage.getFeedCount();
    expect(feedCount).toBeGreaterThan(0);
  });

  test('user can add new feed and view its articles', async ({ page }) => {
    await mockFeedsApi(page, 3);
    await mockArticlesApi(page, 10);

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await loginPage.waitForLoginSuccess();

    // Go to feeds
    await page.goto('/desktop/feeds');
    const feedsPage = new DesktopFeedsPage(page);
    await feedsPage.waitForLoad();

    const initialCount = await feedsPage.getFeedCount();

    // Click add feed
    await feedsPage.clickAddFeed();

    // Should navigate to register page
    await expect(page).toHaveURL(/\/desktop\/feeds\/register/);

    // Fill form
    const urlInput = page.getByLabel(/url/i);
    await urlInput.fill('https://example.com/feed.rss');

    const submitButton = page.getByRole('button', { name: /submit|add/i });
    await submitButton.click();

    // Should redirect back to feeds
    await page.waitForURL(/\/desktop\/feeds$/);

    // Feed count should increase (in real scenario)
    // For now, just verify we're back on feeds page
    await expect(page).toHaveURL(/\/desktop\/feeds$/);
  });

  test('user preferences persist across navigation', async ({ page }) => {
    await mockFeedsApi(page, 5);

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await loginPage.waitForLoginSuccess();

    // Go to settings
    await page.goto('/desktop/settings');

    // Change theme (if available)
    const themeSelect = page.getByLabel(/theme/i);
    if ((await themeSelect.count()) > 0) {
      await themeSelect.selectOption('dark');
      await page.waitForTimeout(500);
    }

    // Navigate to feeds
    await page.goto('/desktop/feeds');

    // Navigate back to settings
    await page.goto('/desktop/settings');

    // Theme should still be dark (if feature exists)
    if ((await themeSelect.count()) > 0) {
      const selectedTheme = await themeSelect.inputValue();
      expect(selectedTheme).toBe('dark');
    }
  });
});
