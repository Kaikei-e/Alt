import { expect, test } from "@playwright/test";
import { mockApiEndpoints, generateMockFeeds } from "../helpers/mockApi";

test.describe("FeedCard Component - Functionality Tests", () => {
  const mockFeeds = generateMockFeeds(10, 1);

  test.beforeEach(async ({ page }) => {
    await mockApiEndpoints(page, { feeds: mockFeeds });
  });

  test.describe("Initial State", () => {
    test("should render feed cards", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 10000 });
    });

    test("should display correct feed titles", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Check for first few feed titles with increased timeout
      await expect(page.getByText("Test Feed 1").first()).toBeVisible({
        timeout: 10000,
      });
      await expect(page.getByText("Test Feed 2").first()).toBeVisible({
        timeout: 10000,
      });
      await expect(page.getByText("Test Feed 3").first()).toBeVisible({
        timeout: 10000,
      });
    });

    test("should display mark as read buttons", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('button:has-text("Mark as read")').first(),
      ).toBeVisible({ timeout: 10000 });
    });

    test("should display correct number of feed cards", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      const feedCards = page.locator('[data-testid="feed-card"]');
      await expect(feedCards).toHaveCount(10);
    });
  });

  test.describe("Feed Content Display", () => {
    test("should display feed descriptions", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.getByText("Description for test feed 1").first(),
      ).toBeVisible();
    });

    test("should display feed links correctly", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      const firstFeedLink = page.locator('a[href="https://example.com/feed1"]');
      await expect(firstFeedLink).toBeVisible();
      await expect(firstFeedLink).toHaveAttribute("target", "_blank");
    });

    test("should handle long descriptions properly", async ({ page }) => {
      // Create a feed with very long description to test truncation
      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        const longDescriptionFeed = {
          ...mockFeeds[0],
          description: "A".repeat(400), // Very long description
        };
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([longDescriptionFeed, ...mockFeeds.slice(1)]),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible();

      // Should show truncated description with ellipsis
      const description = await page
        .locator('[data-testid="feed-card"]')
        .first()
        .locator("text=...");
      await expect(description).toBeVisible();
    });
  });

  test.describe("Feed Interaction", () => {
    test("should be clickable links", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      const feedLink = page.locator('a[href="https://example.com/feed1"]');
      await expect(feedLink).toBeVisible();

      // Check that the link has proper attributes for external navigation
      await expect(feedLink).toHaveAttribute("target", "_blank");
    });

    test("should handle hover states", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      const markAsReadButton = page
        .locator('button:has-text("Mark as read")')
        .first();

      // Hover over the button
      await markAsReadButton.hover();

      // Button should still be visible and functional
      await expect(markAsReadButton).toBeVisible();
    });

    test("should handle focus states", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      const markAsReadButton = page
        .locator('button:has-text("Mark as read")')
        .first();

      // Focus the button
      await markAsReadButton.focus();
      await expect(markAsReadButton).toBeFocused();

      // Should be able to activate with keyboard
      await page.keyboard.press("Enter");

      // Should handle the click (button might disappear if feed is marked as read)
      // Just verify the action was processed
    });
  });

  test.describe("Mark as Read Functionality", () => {
    test("should mark feed as read when button is clicked", async ({
      page,
    }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for feeds to be fully loaded before counting
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 10000 });

      const initialFeedCount = await page
        .locator('button:has-text("Mark as read")')
        .count();

      // Ensure we have feeds to test with
      expect(initialFeedCount).toBeGreaterThan(0);

      const markAsReadButton = page
        .locator('button:has-text("Mark as read")')
        .first();

      await markAsReadButton.click();

      // Wait for the UI to update after marking as read
      await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
        initialFeedCount - 1,
        { timeout: 10000 },
      );
    });

    test("should update UI after marking as read", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for feeds to be properly loaded
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 10000 });

      // First verify the feed is visible
      await expect(page.getByText("Test Feed 1").first()).toBeVisible();

      // Get the specific feed card that contains exactly "Test Feed 1" (not "Test Feed 10")
      const firstFeedCard = page.locator('[data-testid="feed-card"]').filter({
        has: page.getByRole("link", { name: "Test Feed 1", exact: true }),
      });
      const markAsReadButton = firstFeedCard.locator(
        'button:has-text("Mark as read")',
      );

      await markAsReadButton.click();

      // Wait for the mark as read operation to complete and the component to update
      await page.waitForTimeout(1000);

      // The first feed card should no longer be visible
      await expect(firstFeedCard).not.toBeVisible();
    });
  });

  test.describe("Data Validation", () => {
    test("should handle feeds with different content lengths", async ({
      page,
    }) => {
      // Test with feeds of varying content lengths
      const variedFeeds = [
        {
          id: "1",
          title: "Short",
          description: "Short desc",
          link: "https://example.com/short",
          published: "2024-01-01T12:00:00Z",
        },
        {
          id: "2",
          title: "Medium Length Title Here",
          description:
            "This is a medium length description that should display properly in the UI without issues.",
          link: "https://example.com/medium",
          published: "2024-01-02T12:00:00Z",
        },
        {
          id: "3",
          title:
            "Very Long Title That Goes On And On And Should Be Handled Gracefully By The UI",
          description:
            "This is a very long description that contains a lot of text and should be truncated properly by the component to ensure that the UI remains clean and readable even with extensive content that might otherwise break the layout or make it difficult to read other feed items in the list.",
          link: "https://example.com/long",
          published: "2024-01-03T12:00:00Z",
        },
      ];

      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(variedFeeds),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible();

      // All feeds should be displayed
      await expect(page.getByRole("link", { name: "Short" })).toBeVisible();
      await expect(page.getByText("Medium Length Title Here")).toBeVisible();
      await expect(
        page.getByText("Very Long Title That Goes On And On"),
      ).toBeVisible();
    });

    test("should handle empty feed data gracefully", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([]),
        });
      });

      await page.route("**/api/v1/feeds/fetch/list", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([]),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Should show no feeds message
      await expect(page.getByText("No feeds available")).toBeVisible();
    });

    test("should handle API errors gracefully", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Internal server error" }),
        });
      });

      await page.route("**/api/v1/feeds/fetch/list", async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Internal server error" }),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Should show error state
      await expect(page.getByText("Failed to load feeds")).toBeVisible();
    });
  });

  test.describe("Feed Ordering and Display", () => {
    test("should display feeds in correct order", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      // Check that feeds appear in the expected order
      const feedLinks = page.getByRole("link").filter({ hasText: "Test Feed" });

      // First few feeds should be visible in order
      await expect(feedLinks.first()).toHaveText("Test Feed 1");
    });

    test("should handle different feed counts", async ({ page }) => {
      // Test with different number of feeds
      const smallFeedSet = generateMockFeeds(3, 1);

      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(smallFeedSet),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible();

      // Should display only 3 feeds
      const feedCards = page.locator('[data-testid="feed-card"]');
      await expect(feedCards).toHaveCount(3);
    });
  });

  test.describe("Performance and Loading", () => {
    test("should load feeds efficiently", async ({ page }) => {
      const startTime = Date.now();

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible();

      const loadTime = Date.now() - startTime;

      // Should load within reasonable time (20 seconds to account for CI environment variations)
      expect(loadTime).toBeLessThan(20000);
    });

    test("should handle large feed lists", async ({ page }) => {
      // Test with a larger number of feeds
      const largeFeedSet = generateMockFeeds(50, 1);

      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(largeFeedSet.slice(0, 10)), // Still return 10 for first page
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible();

      // Should still display the expected number of feeds
      const feedCards = page.locator('[data-testid="feed-card"]');
      await expect(feedCards).toHaveCount(10);
    });
  });

  test.describe("Responsive Design", () => {
    test("should display properly on mobile viewport", async ({ page }) => {
      // Set mobile viewport
      await page.setViewportSize({ width: 375, height: 667 });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible();

      // Feeds should still be visible and properly formatted
      await expect(
        page.getByRole("link", { name: "Test Feed 1", exact: true }),
      ).toBeVisible();
    });

    test("should display properly on tablet viewport", async ({ page }) => {
      // Set tablet viewport
      await page.setViewportSize({ width: 768, height: 1024 });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible();

      // Feeds should still be visible and properly formatted
      await expect(
        page.getByRole("link", { name: "Test Feed 1", exact: true }),
      ).toBeVisible();
    });
  });

  test.describe("Accessibility", () => {
    test("should have proper link structure", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      const feedLink = page.locator('a[href="https://example.com/feed1"]');

      await expect(feedLink).toBeVisible();
      await expect(feedLink).toHaveAttribute("target", "_blank");
    });

    test("should be keyboard navigable", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      // Tab through interactive elements
      await page.keyboard.press("Tab");

      const focusedElement = page.locator(":focus");
      await expect(focusedElement).toBeVisible();
    });

    test("should have proper semantic structure", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      // Links should be properly structured
      const feedLinks = page.getByRole("link");
      await expect(feedLinks.first()).toBeVisible();
    });

    test("should handle screen reader accessibility", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      // Check that important elements have appropriate text content
      await expect(
        page.getByRole("link", { name: "Test Feed 1", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Mark as read" }).first(),
      ).toBeVisible();
    });
  });
});
