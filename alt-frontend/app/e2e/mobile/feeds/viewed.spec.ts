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

  test("既読記事のみ表示される (PROTECTED)", async ({ page }) => {
    // Wait for data to load
    await expect(
      page.locator('[data-testid="read-feeds-scroll-container"]'),
    ).toBeVisible();

    // Verify page title - use more specific selector to avoid FloatingMenu link
    await expect(
      page.locator('[data-testid="read-feeds-title"]'),
    ).toBeVisible();
    await expect(page.locator('[data-testid="read-feeds-title"]')).toHaveText(
      "Viewed Feeds",
    );

    // Wait for cards to stabilize, then verify count
    await page.waitForTimeout(1000);
    const readFeedCards = page.locator('[data-testid="read-feed-card"]');
    const cardCount = await readFeedCards.count();

    // Expect exactly 2 cards based on our mock
    expect(cardCount).toBe(2);

    // Verify each card shows "Already Read" status
    const readStatusElements = page.getByText("Already Read");
    await expect(readStatusElements).toHaveCount(2);

    // Verify feed titles are displayed
    await expect(page.getByText("Test Read Feed 1")).toBeVisible();
    await expect(page.getByText("Test Read Feed 2")).toBeVisible();

    // Verify feed descriptions are displayed
    await expect(
      page.getByText("This is a test description for read feed 1"),
    ).toBeVisible();
    await expect(
      page.getByText("This is a test description for read feed 2"),
    ).toBeVisible();
  });

  test("Glass効果が適用されている (PROTECTED)", async ({ page }) => {
    // Wait for cards to load and stabilize
    await page.waitForTimeout(1000);

    // Wait for read feed cards to be visible
    const readFeedCard = page.locator('[data-testid="read-feed-card"]').first();
    await expect(readFeedCard).toBeVisible();

    // Verify glass class is applied
    await expect(readFeedCard).toHaveClass(/glass/);

    // Verify glassmorphism visual properties
    const styles = await readFeedCard.evaluate((el) => {
      const computedStyle = getComputedStyle(el);
      return {
        backdropFilter: computedStyle.backdropFilter,
        background: computedStyle.background,
        border: computedStyle.border,
        borderRadius: computedStyle.borderRadius,
      };
    });

    // Verify backdrop filter is applied (glassmorphism effect)
    expect(styles.backdropFilter).toContain("blur");

    // Verify border radius for glass effect
    expect(styles.borderRadius).toBe("16px");

    // Verify theme-appropriate border styling
    const gradientContainer = page
      .locator('[data-testid="read-feed-card-container"]')
      .first();
    const containerStyles = await gradientContainer.evaluate((el) => {
      const computedStyle = getComputedStyle(el);
      return {
        borderRadius: computedStyle.borderRadius,
      };
    });

    // Verify consistent border radius
    expect(containerStyles.borderRadius).toBe("18px");
  });
});
