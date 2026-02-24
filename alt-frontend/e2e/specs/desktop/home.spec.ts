import { test, expect } from "@playwright/test";
import { DesktopHomePage } from "../../pages/desktop/DesktopHomePage";
import { setupAllMocks } from "../../utils/api-mock";

test.describe("Desktop Home", () => {
  let homePage: DesktopHomePage;

  test.beforeEach(async ({ page }) => {
    homePage = new DesktopHomePage(page);
    await setupAllMocks(page);

    // Mock feed stats API
    await page.route("**/v1/feeds/stats*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_amount: { amount: 10 },
          summarized_feed: { amount: 5 },
        }),
      });
    });
  });

  test("should display desktop home page", async () => {
    await homePage.goto();
    await homePage.waitForReady();

    await expect(homePage.dashboardTitle).toBeVisible();
  });

  test("should display sidebar navigation", async () => {
    await homePage.goto();
    await homePage.waitForReady();

    const hasSidebar = await homePage.hasSidebar();
    expect(hasSidebar).toBe(true);
  });

  test("should have navigation links", async () => {
    await homePage.goto();
    await homePage.waitForReady();

    // Check for common navigation items (use first() to avoid strict mode issues)
    await expect(
      homePage.page.getByRole("link", { name: /Dashboard/i }).first(),
    ).toBeVisible();
    await expect(
      homePage.page.getByRole("link", { name: /Feeds/i }).first(),
    ).toBeVisible();
  });

  test("should navigate to feeds page", async () => {
    await homePage.goto();
    await homePage.waitForReady();

    await homePage.navigateToFeeds();

    // Should navigate to feeds page
    await expect(homePage.page).toHaveURL(/\/desktop\/feeds/);
  });

  test("should navigate to settings page", async () => {
    await homePage.goto();
    await homePage.waitForReady();

    await homePage.navigateToSettings();

    // Should navigate to settings page
    await expect(homePage.page).toHaveURL(/\/desktop\/settings/);
  });
});
