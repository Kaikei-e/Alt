import { test, expect } from "@playwright/test";
import { FeedDetails, Feed } from "@/schema/feed";

const generateMockFeeds = (count: number, startId: number = 1): Feed[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Feed ${startId + index}`,
    description: `Description for test feed ${startId + index}. This is a longer description to test how the UI handles different text lengths.`,
    link: `https://example.com/feed${startId + index}`,
    published: `2024-01-${String(index + 1).padStart(2, "0")}T12:00:00Z`,
  }));
};

const generateMockFeedDetails = (
  count: number,
  startId: number = 1,
): FeedDetails[] => {
  return Array.from({ length: count }, (_, index) => ({
    feed_url: `https://example.com/feed${index + 1}`,
    summary: `Test Summary for feed ${index + 1}`,
  }));
};

test.describe("FeedDetails", () => {
  test("should show and hide details", async ({ page }) => {
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(generateMockFeeds(10, 1)),
      });
    });

    await page.route("**/api/v1/feeds/fetch/details", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_url: "https://example.com/feed1",
          summary: "Test Summary for feed 1"
        }),
      });
    });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");
    await expect(page.getByTestId("show-details-button").first()).toBeVisible();

    await page.getByTestId("show-details-button").first().click();
    await expect(page.getByTestId("hide-details-button")).toBeVisible();
    await expect(page.getByTestId("summary-text")).toBeVisible();

    await page.getByTestId("hide-details-button").click();
    await expect(page.getByTestId("show-details-button").first()).toBeVisible();
    await expect(page.getByTestId("summary-text")).not.toBeVisible();
  });

  test("should show details when toggle button is clicked", async ({
    page,
  }) => {
    await page.route("**/api/v1/feeds/fetch/details", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_url: "https://example.com/feed1",
          summary: "Test Summary"
        }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(generateMockFeeds(1, 1)),
      });
    });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");
    await page.getByTestId("show-details-button").first().click();

    await expect(page.getByTestId("show-details-button")).not.toBeVisible();
    await expect(page.getByTestId("hide-details-button")).toBeVisible();

    await expect(page.getByTestId("summary-text")).toBeVisible();
  });
});
