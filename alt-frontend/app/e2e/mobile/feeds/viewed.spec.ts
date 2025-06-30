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

  test("アクセシビリティが正しく実装されている (PROTECTED)", async ({
    page,
  }) => {
    // Wait for cards to load
    await page.waitForTimeout(1000);

    // Verify ARIA labels and roles
    const readFeedCard = page.locator('[data-testid="read-feed-card"]').first();
    await expect(readFeedCard).toHaveAttribute("role", "article");
    await expect(readFeedCard).toHaveAttribute(
      "aria-label",
      "Read feed: Test Read Feed 1",
    );

    // Verify live region for screen readers - should be visually hidden but present in DOM
    const liveRegion = page.locator('[aria-live="polite"]');
    await expect(liveRegion).toBeAttached(); // Check it exists in DOM

    // Check it's visually hidden with proper CSS
    const liveRegionStyles = await liveRegion.evaluate((el) => {
      const styles = getComputedStyle(el);
      return {
        position: styles.position,
        left: styles.left,
        width: styles.width,
        height: styles.height,
        overflow: styles.overflow,
        clip: styles.clip,
        visibility: styles.visibility,
      };
    });

    // Verify it's properly hidden via CSS positioning
    expect(liveRegionStyles.position).toBe("absolute");
    expect(parseInt(liveRegionStyles.left)).toBeLessThan(-1000);

    // Verify keyboard navigation for feed links - use first() to avoid strict mode violation
    const feedLink = page
      .getByRole("link", {
        name: "Open read feed Test Read Feed 1 in external link",
      })
      .first();
    await expect(feedLink).toBeVisible();

    // Test keyboard focus
    await feedLink.focus();
    await expect(feedLink).toBeFocused();

    // Verify external link attributes
    await expect(feedLink).toHaveAttribute("target", "_blank");
    await expect(feedLink).toHaveAttribute("rel", "noopener noreferrer");
  });

  test("エラー状態とリトライ機能が動作する (PROTECTED)", async ({ page }) => {
    // Clear existing routes and override with error response
    await page.unroute("**/v1/feeds/fetch/viewed/cursor**");
    await page.route("**/v1/feeds/fetch/viewed/cursor**", async (route) => {
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "Internal Server Error" }),
      });
    });

    // Navigate to trigger error
    await page.goto("/mobile/feeds/viewed");

    // Wait for error state to appear
    await expect(page.getByText(/error/i)).toBeVisible();

    // Verify retry button is present
    const retryButton = page.getByRole("button", { name: /retry/i });
    await expect(retryButton).toBeVisible();

    // Mock successful retry
    await page.route("**/v1/feeds/fetch/viewed/cursor**", async (route) => {
      const mockFeeds = {
        data: [
          {
            title: "Test Read Feed After Retry",
            description: "This feed loaded after retry",
            link: "https://example.com/retry-feed",
            published: "2024-01-01T00:00:00Z",
          },
        ],
        next_cursor: null,
      };

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mockFeeds),
      });
    });

    // Click retry button
    await retryButton.click();

    // Verify successful retry
    await expect(page.getByText("Test Read Feed After Retry")).toBeVisible();
  });

  test("空の状態が正しく表示される (PROTECTED)", async ({ page }) => {
    // Clear existing routes and mock empty response
    await page.unroute("**/v1/feeds/fetch/viewed/cursor**");
    await page.route("**/v1/feeds/fetch/viewed/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [],
          next_cursor: null,
        }),
      });
    });

    await page.goto("/mobile/feeds/viewed");
    await page.waitForLoadState("networkidle");

    // Verify empty state message
    await expect(page.getByText("No read feeds yet")).toBeVisible();
    await expect(
      page.getByText("Mark some feeds as read to see them here"),
    ).toBeVisible();

    // Verify glass effect on empty state
    const emptyStateContainer = page
      .locator(".glass")
      .filter({ hasText: "No read feeds yet" });
    await expect(emptyStateContainer).toBeVisible();
  });
});
