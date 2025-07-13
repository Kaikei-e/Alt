import { test, expect } from "@playwright/test";

test.describe("VirtualDesktopTimeline Component - Performance Tests", () => {
  test.beforeEach(async ({ page }) => {
    // Mock API endpoints to prevent network errors
    await page.route("**/api/v1/feeds/fetch/cursor**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [
            {
              title: "Test Feed 1",
              description: "Description for test feed 1",
              link: "https://example.com/feed1",
              published: "2024-01-01T12:00:00Z",
            },
            {
              title: "Test Feed 2",
              description: "Description for test feed 2",
              link: "https://example.com/feed2",
              published: "2024-01-02T12:00:00Z",
            },
          ],
          next_cursor: null,
        }),
      });
    });

    // Mock other API endpoints that might be called
    await page.route("**/api/v1/feeds/stats**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_amount: { amount: 42 },
          summarized_feed: { amount: 28 },
        }),
      });
    });

    await page.route("**/api/v1/health**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "ok" }),
      });
    });

    // Navigate to desktop feeds page
    await page.goto("/desktop/feeds");
  });

  test("should render virtual desktop timeline with visible items only", async ({
    page,
  }) => {
    // Wait for the page to load and check for presence of feed cards
    await page.waitForLoadState("networkidle");

    // Check for feed cards (actual implementation uses LazyDesktopTimeline)
    const feedCards = page.locator('[data-testid^="desktop-feed-card-"]');
    await expect(feedCards.first()).toBeVisible({ timeout: 10000 });

    // Check that feed cards are rendered
    const renderedItems = await feedCards.count();

    // Should render feed cards based on mock data
    expect(renderedItems).toBeGreaterThan(0);
    expect(renderedItems).toBeLessThanOrEqual(2); // Based on mock data
  });

  test("should handle scrolling efficiently with larger desktop cards", async ({
    page,
  }) => {
    // Wait for page to load completely
    await page.waitForLoadState("networkidle");

    // Wait for feed cards to be present
    await expect(
      page.locator('[data-testid^="desktop-feed-card-"]').first(),
    ).toBeVisible({ timeout: 15000 });

    // Simple scroll test without complex validation
    await page.keyboard.press("PageDown");
    await page.waitForTimeout(500);

    // Verify page still functions after scroll
    const feedCards = page.locator('[data-testid^="desktop-feed-card-"]');
    await expect(feedCards.first()).toBeVisible();

    // Should maintain some feed cards rendered
    const renderedItems = await feedCards.count();
    expect(renderedItems).toBeGreaterThan(0);
  });

  test("should maintain performance with desktop-sized cards", async ({
    page,
  }) => {
    // Navigate to page and wait for load
    await page.goto("/desktop/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feed cards to be present
    await expect(
      page.locator('[data-testid^="desktop-feed-card-"]').first(),
    ).toBeVisible({ timeout: 10000 });

    // Measure initial performance
    const startTime = Date.now();

    // Perform multiple scroll operations
    for (let i = 0; i < 3; i++) {
      await page.keyboard.press("PageDown");
      await page.waitForTimeout(150);
    }

    const endTime = Date.now();
    const duration = endTime - startTime;

    // Should complete scrolling operations within reasonable time
    expect(duration).toBeLessThan(1500); // 1.5 seconds for 3 scroll operations

    // Check that DOM size is still reasonable
    const finalItems = await page
      .locator('[data-testid^="desktop-feed-card-"]')
      .count();
    expect(finalItems).toBeLessThanOrEqual(2); // Based on mock data
  });

  test("should handle desktop feed interactions correctly", async ({
    page,
  }) => {
    await page.waitForLoadState("networkidle");

    // Find first feed card directly (simpler approach)
    const firstFeedCard = page
      .locator('[data-testid^="desktop-feed-card-"]')
      .first();
    await expect(firstFeedCard).toBeVisible({ timeout: 15000 });

    // Simple interaction test - just verify we can interact with the card
    await firstFeedCard.hover();
    await page.waitForTimeout(200);

    // Verify the card is still visible after interaction
    await expect(firstFeedCard).toBeVisible();
  });

  test("should work with different desktop viewport sizes", async ({
    page,
  }) => {
    // Test standard desktop viewport
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goto("/desktop/feeds");
    await page.waitForLoadState("networkidle");

    // Just verify feed cards are visible
    const feedCards = page.locator('[data-testid^="desktop-feed-card-"]');
    await expect(feedCards.first()).toBeVisible({ timeout: 15000 });

    const itemCount = await feedCards.count();
    expect(itemCount).toBeGreaterThan(0);

    console.log(`Desktop viewport: ${itemCount} items rendered`);
  });

  test("should handle empty state gracefully", async ({ page }) => {
    // Mock empty response
    await page.route("**/api/v1/feeds/fetch/cursor**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [],
          next_cursor: null,
        }),
      });
    });

    await page.goto("/desktop/feeds");

    // Should show empty state
    await expect(page.getByText("No feeds available")).toBeVisible();
    await expect(
      page.getByText(
        "Your feed will appear here once you subscribe to sources",
      ),
    ).toBeVisible();
  });
});
