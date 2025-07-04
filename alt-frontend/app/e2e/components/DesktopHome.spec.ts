import { test, expect } from "@playwright/test";

test.describe("DesktopHome Unread Count", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/feeds/stats", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_amount: { amount: 1 },
          summarized_feed: { amount: 1 },
        }),
      });
    });
    await page.route("**/api/v1/feeds/count/unreads**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ count: 5 }),
      });
    });
    await page.goto("/desktop/home");
    await page.waitForLoadState("networkidle");
  });

  test("should display unread count", async ({ page }) => {
    await expect(page.getByText("Unread Articles")).toBeVisible();
    await expect(page.getByText("5")).toBeVisible();
  });
});
