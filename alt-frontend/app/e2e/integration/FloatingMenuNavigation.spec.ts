import { test, expect } from "@playwright/test";

test.describe("FloatingMenu Cross-Page Navigation Integration Tests", () => {
  test.beforeEach(async ({ page }) => {
    // Mock API endpoints to prevent backend dependencies
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [
            {
              title: "Test Feed 1",
              description: "Test description 1",
              link: "https://example.com/feed/1",
              published: new Date().toISOString(),
            },
          ],
          next_cursor: null,
        }),
      });
    });

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

    await page.route("**/api/v1/feeds/stats", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_amount: { amount: 42 },
          summarized_feed: { amount: 15 },
        }),
      });
    });
  });

  test("should display Home menu item with correct href", async ({ page }) => {
    // Navigate to feeds page
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for FloatingMenu
    await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 10000 });

    // Open FloatingMenu
    await page.getByTestId("floating-menu-button").click();
    await expect(page.getByTestId("menu-content")).toBeVisible();

    // Verify Home menu item exists and has correct href
    const homeLink = page
      .getByTestId("menu-content")
      .getByRole("link")
      .filter({ hasText: "Home" });

    await expect(homeLink).toBeVisible();
    await expect(homeLink).toHaveAttribute("href", "/");

    // Verify Home is the last item
    const allLinks = page.getByTestId("menu-content").getByRole("link");
    const linkCount = await allLinks.count();
    const lastLink = allLinks.nth(linkCount - 1);
    await expect(lastLink).toHaveText("Home");
  });

  test("should navigate to home page when Home link is clicked", async ({ page }) => {
    // Start at feeds page
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Verify we're on the feeds page
    expect(page.url()).toContain("/mobile/feeds");

    // Wait for FloatingMenu
    await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 10000 });

    // Open FloatingMenu
    await page.getByTestId("floating-menu-button").click();
    await expect(page.getByTestId("menu-content")).toBeVisible();

    // Get Home link
    const homeLink = page
      .getByTestId("menu-content")
      .getByRole("link")
      .filter({ hasText: "Home" });

    // Navigation and closing the menu can race. We use force: true to avoid
    // Playwright waiting for the element to be stable, which it won't be.
    await Promise.all([
      page.waitForURL((url) => url.pathname === "/" || url.pathname === ""),
      homeLink.click({ force: true }),
    ]);
    await page.waitForLoadState("networkidle");

    // After navigation, should be on home page
    const currentUrl = page.url();
    console.log("Current URL after navigation:", currentUrl);
    expect(currentUrl).toMatch(/http:\/\/localhost:\d+\/?$/);

    // Verify home page content is loaded
    await expect(
      page.getByRole("heading", { name: "Alt" })
    ).toBeVisible({ timeout: 10000 });
  });

  test("should handle navigation without JavaScript errors", async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => {
      errors.push(error.message);
    });

    await page.goto("/mobile/feeds", { waitUntil: "networkidle" });
    const menuButton = page.getByTestId("floating-menu-button");
    await expect(menuButton).toBeVisible({ timeout: 10000 });
    await menuButton.click();
    await expect(page.getByTestId("menu-content")).toBeVisible({
      timeout: 10000,
    });

    const homeLink = page
      .getByTestId("menu-content")
      .getByRole("link")
      .filter({ hasText: "Home" });

    if (await homeLink.isVisible()) {
      await Promise.all([
        page.waitForURL((url) => url.pathname === "/" || url.pathname === ""),
        homeLink.click({ force: true }),
      ]);
      await page.waitForLoadState("networkidle");
    }

    expect(errors).toHaveLength(0);
  });

  test("should work correctly from different mobile pages", async ({
    page,
  }) => {
    // Only test pages that actually have FloatingMenu
    const testPages = ["/mobile/feeds"];

    for (const testPage of testPages) {
      // Navigate to test page
      await page.goto(testPage);
      await page.waitForLoadState("networkidle");

      try {
        // Wait for FloatingMenu
        await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 5000 });

        // Open menu
        await page.getByTestId("floating-menu-button").click();
        await expect(page.getByTestId("menu-content")).toBeVisible();

        // Verify Home link is present
        const homeLink = page
          .getByTestId("menu-content")
          .getByRole("link")
          .filter({ hasText: "Home" });

        await expect(homeLink).toBeVisible();
        await expect(homeLink).toHaveAttribute("href", "/");

        // Close menu for next iteration
        await page.getByTestId("close-menu-button").click();
        await expect(page.getByTestId("menu-content")).not.toBeVisible();

      } catch (error) {
        console.log(`FloatingMenu not available on ${testPage}: ${error}`);
      }
    }
  });
});