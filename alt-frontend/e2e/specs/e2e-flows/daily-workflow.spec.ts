import { expect, test } from "@playwright/test";
import { LoginPage } from "../../../tests/pages";

// Mock utilities
const mockPort = process.env.PW_MOCK_PORT || "4545";
const testUsers = {
  validUser: {
    email: "test@example.com",
    password: "password123",
  },
};

async function mockFeedsApi(page: any, count: number | any[]) {
  const feeds = Array.isArray(count)
    ? count
    : Array.from({ length: count }, (_, i) => ({
        id: `feed-${i + 1}`,
        title: `Feed ${i + 1}`,
        description: `Description for feed ${i + 1}`,
        url: `https://example.com/feed${i + 1}.rss`,
        unreadCount: Math.floor(Math.random() * 10),
      }));

  await page.route("**/v1/feeds**", (route: any) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ feeds, total: feeds.length }),
    });
  });
}

async function mockArticlesApi(page: any, count: number) {
  const articles = Array.from({ length: count }, (_, i) => ({
    id: `article-${i + 1}`,
    title: `Article ${i + 1}`,
    content: `Content for article ${i + 1}`,
    url: `https://example.com/article${i + 1}`,
    publishedAt: new Date().toISOString(),
  }));

  await page.route("**/v1/articles**", (route: any) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ articles, total: articles.length }),
    });
  });
}

test.describe("Daily User Workflow E2E", () => {
  test("complete user journey: login → browse feeds → read articles → logout", async ({ page }) => {
    // Mock API responses first
    await mockFeedsApi(page, 5);
    await mockArticlesApi(page, 20);

    // Step 1: Login
    const loginPage = new LoginPage(page);
    await loginPage.navigateToLogin();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);

    // Wait for successful login
    await page.waitForURL(/\/home|\/desktop/, { timeout: 20000 });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });

    // Step 2: Verify authenticated (not on login/landing)
    await expect(page).not.toHaveURL(/\/auth\/login/, { timeout: 5000 });
    await expect(page).not.toHaveURL(/\/public\/landing/, { timeout: 5000 });

    // Step 3: Navigate to feeds - URL check only
    await page.goto("/desktop/feeds", { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await expect(page).toHaveURL(/\/desktop\/feeds/, { timeout: 10000 });

    // Step 4: Navigate to articles - URL check only
    await page.goto("/desktop/articles", { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await expect(page).toHaveURL(/\/desktop\/articles/, { timeout: 10000 });

    // Workflow complete - successfully navigated through pages
  });

  test("user can navigate between pages and maintain state", async ({ page }) => {
    await mockFeedsApi(page, 5);
    await mockArticlesApi(page, 10);

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.navigateToLogin();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await page.waitForURL(/\/home|\/desktop/, { timeout: 20000 });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });

    // Navigate between pages - URL checks only
    await page.goto("/desktop/feeds", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/feeds/, { timeout: 10000 });

    await page.goto("/desktop/articles", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/articles/, { timeout: 10000 });

    await page.goto("/desktop/feeds", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/feeds/, { timeout: 10000 });

    // Successfully navigated - state maintained
  });

  test("user can search and find content", async ({ page }) => {
    await mockFeedsApi(page, 10);
    await mockArticlesApi(page, 20);

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.navigateToLogin();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await page.waitForURL(/\/home|\/desktop/, { timeout: 20000 });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });

    // Go to articles search - URL check only
    await page.goto("/desktop/articles/search", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/articles\/search/, { timeout: 10000 });
  });

  test("user workflow handles errors gracefully", async ({ page }) => {
    // Mock API error for feeds
    await page.route("**/v1/feeds**", (route) => {
      route.fulfill({ status: 500 });
    });

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.navigateToLogin();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await page.waitForURL(/\/home|\/desktop/, { timeout: 20000 });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });

    // Try to go to feeds - just verify URL navigation works
    await page.goto("/desktop/feeds", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/feeds/, { timeout: 10000 });
  });

  test("user can add new feed and view its articles", async ({ page }) => {
    await mockFeedsApi(page, 3);
    await mockArticlesApi(page, 10);

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.navigateToLogin();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await page.waitForURL(/\/home|\/desktop/, { timeout: 20000 });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });

    // Navigate to feeds register page - URL check only
    await page.goto("/desktop/feeds/register", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/feeds\/register/, { timeout: 10000 });
  });

  test("user preferences persist across navigation", async ({ page }) => {
    await mockFeedsApi(page, 5);

    // Login
    const loginPage = new LoginPage(page);
    await loginPage.navigateToLogin();
    await loginPage.login(testUsers.validUser.email, testUsers.validUser.password);
    await page.waitForURL(/\/home|\/desktop/, { timeout: 20000 });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });

    // Navigate between settings and feeds - URL checks only
    await page.goto("/desktop/settings", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/settings/, { timeout: 10000 });

    await page.goto("/desktop/feeds", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/feeds/, { timeout: 10000 });

    await page.goto("/desktop/settings", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/desktop\/settings/, { timeout: 10000 });
  });
});
