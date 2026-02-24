import { expect, test } from "@playwright/test";
import { MobileStatsPage } from "../../pages/mobile/MobileStatsPage";
import { setupAllMocks } from "../../utils/api-mock";

test.describe("Mobile Feed Stats", () => {
  let statsPage: MobileStatsPage;

  test.beforeEach(async ({ page }) => {
    statsPage = new MobileStatsPage(page);
    await setupAllMocks(page);

    // Mock SSE endpoint for feed stats
    await page.route("**/v1/feeds/stats/sse*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: 'data: {"feed_amount":10,"unsummarized_articles_amount":5,"total_articles_amount":100}\n\n',
      });
    });
  });

  test("should display stats page", async () => {
    await statsPage.goto();
    await statsPage.waitForReady();

    await expect(statsPage.statsHeading).toBeVisible();
    await expect(statsPage.statsHeading).toHaveText("Feeds Statistics");
  });

  test("should display stats cards", async () => {
    await statsPage.goto();
    await statsPage.waitForReady();

    const hasCards = await statsPage.hasStatsCards();
    expect(hasCards).toBe(true);
  });

  test("should show connection status", async () => {
    await statsPage.goto();
    await statsPage.waitForReady();

    // Connection status should be visible
    await expect(statsPage.connectionStatus).toBeVisible();
  });
});
