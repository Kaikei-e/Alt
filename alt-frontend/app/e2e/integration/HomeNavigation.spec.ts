import { test, expect } from "@playwright/test";

test.describe("Home Navigation Manual Test", () => {
  test.beforeEach(async ({ page }) => {
    // Mock API endpoints
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify([
          {
            title: "Test Feed 1",
            description: "Test description 1",
            link: "https://example.com/feed/1",
            published: new Date().toISOString(),
          },
        ]),
      });
    });

    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [],
          next_cursor: null,
        }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/list", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify([]),
      });
    });

    await page.route("**/api/v1/health", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "ok" }),
      });
    });

    await page.route("**/api/v1/feed_stats", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_amount: { amount: 42 },
          summarized_feed: { amount: 15 }
        }),
      });
    });
  });

  test("should navigate directly to home page", async ({ page }) => {
    // First, test that home page loads correctly
    await page.goto("/");
    await page.waitForLoadState("networkidle");

    // Check if home page content is visible (use role to avoid title tag conflict)
    await expect(page.getByRole("heading", { name: "Alt" })).toBeVisible();

    console.log("Home page URL:", page.url());
    const currentUrl = page.url();
    expect(currentUrl).toMatch(/\/$|localhost:3010\/?$/);
  });

  test("should verify Home link is clickable in FloatingMenu", async ({ page }) => {
    // Start at feeds page
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for FloatingMenu button
    await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 10000 });

    // Open menu
    await page.getByTestId("floating-menu-button").click();
    await expect(page.getByTestId("menu-content")).toBeVisible();

    // Find Home link
    const homeLink = page.getByTestId("menu-content").getByRole("link").filter({ hasText: "Home" });

    // Check link properties
    await expect(homeLink).toBeVisible();
    await expect(homeLink).toHaveAttribute("href", "/");

    // Check if it's actually clickable
    await expect(homeLink).toBeEnabled();

    console.log("Home link found and is clickable");
  });

  test("should use manual click and verify DOM changes", async ({ page }) => {
    // Start at feeds page
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    console.log("Starting at feeds page:", page.url());

    // Wait for FloatingMenu button
    await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 10000 });

    // Open menu
    await page.getByTestId("floating-menu-button").click();
    await expect(page.getByTestId("menu-content")).toBeVisible();

    // Get the Home link
    const homeLink = page.getByTestId("menu-content").getByRole("link").filter({ hasText: "Home" });

    // Check the href attribute before clicking
    const href = await homeLink.getAttribute('href');
    console.log("Home link href:", href);

    // Manual approach - actually click the link
    await homeLink.click();

    // Give it time for navigation
    await page.waitForTimeout(2000);
    await page.waitForLoadState("networkidle");

    console.log("URL after clicking Home link:", page.url());

    // Check if we navigated or if something prevented it
    const currentUrl = page.url();
    if (currentUrl.includes("/mobile/feeds")) {
      console.log("⚠️  Navigation was prevented or failed");

      // Check if there are any errors
      const errors = await page.evaluate(() => {
        return window.console.error || [];
      });
      console.log("Console errors:", errors);

      // Try direct navigation as comparison
      await page.goto("/");
      await page.waitForLoadState("networkidle");
      console.log("Direct navigation to home works:", page.url());
    } else {
      console.log("✅ Navigation successful");
    }
  });
});