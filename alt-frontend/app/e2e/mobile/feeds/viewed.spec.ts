import { test, expect } from "@playwright/test";

test.describe("既読記事ページ - PROTECTED", () => {
  test.beforeEach(async ({ page }) => {
    // Clear specific routes to avoid interference
    await page.unroute("**/v1/feeds/fetch/viewed/cursor**");

    // Mock API responses for consistent testing
    await page.route("**/v1/feeds/fetch/viewed/cursor**", async (route) => {
      const url = route.request().url();

      // For initial load, return exactly 2 feeds
      if (!url.includes("cursor=")) {
        const mockFeeds = {
          data: [
            {
              title: "Test Read Feed 1",
              description: "This is a test description for read feed 1",
              link: "https://example.com/feed1",
              published: "2024-01-01T00:00:00Z",
            },
            {
              title: "Test Read Feed 2",
              description: "This is a test description for read feed 2",
              link: "https://example.com/feed2",
              published: "2024-01-02T00:00:00Z",
            },
          ],
          next_cursor: "next-cursor-token",
        };

        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(mockFeeds),
        });
      } else {
        // For pagination, return empty to end pagination
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], next_cursor: null }),
        });
      }
    });

    // Mock empty state scenario for specific tests
    await page.route(
      "**/v1/feeds/fetch/viewed/cursor?empty=true",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], next_cursor: null }),
        });
      },
    );

    await page.goto("/mobile/feeds/viewed");
    await page.waitForLoadState("networkidle");
  });
});
