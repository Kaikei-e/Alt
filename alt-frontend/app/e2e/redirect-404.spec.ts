import { test, expect } from "@playwright/test";

test("redirects unknown paths to home", async ({ page }) => {
  await page.route("**/api/v1/feeds/stats", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        feed_amount: { amount: 0 },
        summarized_feed: { amount: 0 },
      }),
    });
  });

  await page.goto("/non-existent-page");
  await page.waitForLoadState("networkidle");
  await expect(page).toHaveURL("/");
});
