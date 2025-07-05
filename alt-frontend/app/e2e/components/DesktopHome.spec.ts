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
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: [], next_cursor: null }),
      });
    });
  });

  test("should display unread count", async ({ page }) => {
    await page.goto("/desktop/home");

    // Wait for the unread count to be visible
    await expect(page.getByText("Unread Articles")).toBeVisible();

    // Find the unread count by locating the number near "Unread Articles" text
    const unreadSection = page.locator('text="Unread Articles"').locator("..");
    await expect(unreadSection.getByText("5").first()).toBeVisible();
  });
});
