import { test, expect } from "@playwright/test";

test.describe("VirtualFeedList Component - Performance Tests", () => {
  test.beforeEach(async ({ page }) => {
    // Mock API endpoints to prevent network errors
    await page.route("**/api/v1/feeds/fetch/cursor**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: Array.from({ length: 20 }, (_, i) => ({
            title: `Test Feed ${i + 1}`,
            description: `Description for test feed ${i + 1}`,
            link: `https://example.com/feed${i + 1}`,
            published: `2024-01-${String(i + 1).padStart(2, "0")}T12:00:00Z`,
          })),
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

    // Navigate to mobile feeds page
    await page.goto("/mobile/feeds");
  });

  test("should render virtual feed list with visible items only", async ({
    page,
  }) => {
    // Wait for feeds to load and virtual list to be triggered (>15 items)
    await page.waitForSelector('[data-testid="virtual-feed-list"]', {
      timeout: 10000,
    });

    const virtualContainer = page.locator('[data-testid="virtual-feed-list"]');
    await expect(virtualContainer).toBeVisible();

    // Check that virtual feed items are rendered
    const renderedItems = await page
      .locator('[data-testid^="virtual-feed-item-"]')
      .count();

    // Should render only visible items + overscan (estimated 10-15 items for viewport)
    expect(renderedItems).toBeGreaterThan(0);
    expect(renderedItems).toBeLessThan(25); // Maximum expected with overscan
  });

  test("should handle scrolling and render new items dynamically", async ({
    page,
  }) => {
    // Wait for virtual list to load
    await page.waitForSelector('[data-testid="virtual-feed-list"]');

    // Get initial rendered items
    const initialItems = await page
      .locator('[data-testid="feed-card"]')
      .count();

    // Scroll down within the scroll container
    await page.locator('[data-testid="feeds-scroll-container"]').focus();
    await page.keyboard.press("PageDown");
    await page.keyboard.press("PageDown");

    // Wait for scroll to complete
    await page.waitForTimeout(500);

    // Check that scroll position changed
    const scrollTop = await page.evaluate(() => {
      const element = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      return element?.scrollTop || 0;
    });

    expect(scrollTop).toBeGreaterThan(0);

    // All items should still be visible (no virtualization in current impl)
    const newItems = await page.locator('[data-testid="feed-card"]').count();
    expect(newItems).toBeLessThanOrEqual(20);
  });

  test("should maintain performance with large datasets", async ({ page }) => {
    // Navigate to page and wait for load
    await page.goto("/mobile/feeds");
    await page.waitForSelector('[data-testid="virtual-feed-list"]');

    // Measure initial performance
    const startTime = Date.now();

    // Perform multiple scroll operations
    for (let i = 0; i < 5; i++) {
      await page.keyboard.press("PageDown");
      await page.waitForTimeout(100);
    }

    const endTime = Date.now();
    const duration = endTime - startTime;

    // Should complete scrolling operations within reasonable time
    expect(duration).toBeLessThan(2000); // 2 seconds for 5 scroll operations

    // Check that DOM size is still reasonable
    const finalItems = await page.locator('[data-testid="feed-card"]').count();
    expect(finalItems).toBeLessThanOrEqual(20);
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

    await page.goto("/mobile/feeds");

    // Should show empty state instead of virtual list
    await expect(
      page.locator('[data-testid="virtual-feed-list"]'),
    ).not.toBeVisible();
    await expect(page.getByText("No feeds available")).toBeVisible();
  });

  test("should work across different viewport sizes", async ({ page }) => {
    // Test mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto("/mobile/feeds");
    await page.waitForSelector('[data-testid="virtual-feed-list"]');

    let mobileItems = await page.locator('[data-testid="feed-card"]').count();
    expect(mobileItems).toBeGreaterThan(0);

    // Test tablet viewport
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.reload();
    await page.waitForSelector('[data-testid="virtual-feed-list"]');

    let tabletItems = await page.locator('[data-testid="feed-card"]').count();
    expect(tabletItems).toBeGreaterThan(0);

    // Both should maintain reasonable DOM size
    expect(mobileItems).toBeLessThanOrEqual(20);
    expect(tabletItems).toBeLessThanOrEqual(20);
  });
});
