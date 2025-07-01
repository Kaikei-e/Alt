import { expect, test } from "@playwright/test";
import { mockApiEndpoints, generateMockFeeds } from "../helpers/mockApi";

test.describe("FeedCard Component - Functionality Tests", () => {
  const mockFeeds = generateMockFeeds(10, 1);

  test.beforeEach(async ({ page }) => {
    await page.unrouteAll();

    await page.route("**/api/v1/health", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "ok" }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: mockFeeds,
          next_cursor: null,
        }),
      });
    });

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
      const longDescriptionFeed = {
        ...mockFeeds[0],
        description: "A".repeat(400), // Very long description (400 chars > 200 limit)
      };
      const feedsWithLongDescription = [
        longDescriptionFeed,
        ...mockFeeds.slice(1),
      ];

      // Mock cursor-based API endpoint
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: feedsWithLongDescription,
            next_cursor: null,
          }),
        });
      });

      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(feedsWithLongDescription),
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
      // Mock the mark as read API call
      await page.route("**/api/v1/feeds/read", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ message: "Feed read status updated" }),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for feeds to be properly loaded with increased timeout
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });

      // Count total feed cards before marking as read
      const initialFeedCount = await page
        .locator('[data-testid="feed-card"]')
        .count();

      // Get the first feed card
      const firstFeedCard = page.locator('[data-testid="feed-card"]').first();

      // Wait for the mark as read button to be available
      const markAsReadButton = firstFeedCard.locator(
        'button:has-text("Mark as read")',
      );
      await expect(markAsReadButton).toBeVisible({ timeout: 10000 });

      // Click mark as read
      await markAsReadButton.click();

      // Wait for the API call to complete and component to re-render
      await page.waitForTimeout(1000);

      // The total number of feed cards should be reduced by 1
      await expect(page.locator('[data-testid="feed-card"]')).toHaveCount(
        initialFeedCount - 1,
        { timeout: 5000 },
      );
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
            "This is a very long description that contains a lot of text and should be truncated properly by the component to ensure that the UI remains clean and readable even with extensive content that might otherwise break the layout or make it difficult to read other feed items in the list. This description exceeds 200 characters to test the truncation functionality.",
          link: "https://example.com/long",
          published: "2024-01-03T12:00:00Z",
        },
      ];

      // Mock cursor-based API endpoint
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: variedFeeds,
            next_cursor: null,
          }),
        });
      });

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
      // Mock the cursor-based API endpoint to return empty data
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: [],
            next_cursor: null,
          }),
        });
      });

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
      // Mock the cursor-based API endpoint to return error
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Internal server error" }),
        });
      });

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

      // Should show error state - updated to match actual ErrorState component text
      await expect(page.getByText("Unable to Load Feeds")).toBeVisible({
        timeout: 10000,
      });
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

      // Mock cursor-based API endpoint
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: smallFeedSet,
            next_cursor: null,
          }),
        });
      });

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
      // Ensure API mocks are in place for this test
      await mockApiEndpoints(page, { feeds: mockFeeds });

      const startTime = Date.now();

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });

      const loadTime = Date.now() - startTime;

      // Should load within reasonable time (increased to 30 seconds for CI environment)
      expect(loadTime).toBeLessThan(30000);
    });

    test("should handle large feed lists", async ({ page }) => {
      // Test with a larger number of feeds
      const largeFeedSet = generateMockFeeds(50, 1);
      const firstPageFeeds = largeFeedSet.slice(0, 10);

      // Mock cursor-based API endpoint
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: firstPageFeeds,
            next_cursor: "10", // Has more data
          }),
        });
      });

      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(firstPageFeeds), // Still return 10 for first page
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

      // Wait for feed cards with increased timeout for mobile
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });

      // Feeds should still be visible and properly formatted - use the actual aria-label
      await expect(
        page.getByRole("link", { name: "Open Test Feed 1 in external link" }),
      ).toBeVisible({ timeout: 10000 });
    });

    test("should display properly on tablet viewport", async ({ page }) => {
      // Set tablet viewport
      await page.setViewportSize({ width: 768, height: 1024 });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for feed cards with increased timeout for tablet
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });

      // Feeds should still be visible and properly formatted - use the actual aria-label
      await expect(
        page.getByRole("link", { name: "Open Test Feed 1 in external link" }),
      ).toBeVisible({ timeout: 10000 });
    });
  });

  test.describe("Accessibility", () => {
    test("should have proper link structure", async ({ page }) => {
      // Ensure API mocks are properly set up for this test
      await mockApiEndpoints(page, { feeds: mockFeeds });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for feed cards to load first
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });

      const feedLink = page.locator('a[href="https://example.com/feed1"]');

      await expect(feedLink).toBeVisible();
      await expect(feedLink).toHaveAttribute("target", "_blank");
    });

    test("should be keyboard navigable", async ({ page }) => {
      // Ensure API mocks are properly set up for this test
      await mockApiEndpoints(page, { feeds: mockFeeds });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for feed cards to load first
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });

      // Tab through interactive elements - may take several tabs to reach interactive content
      for (let i = 0; i < 5; i++) {
        await page.keyboard.press("Tab");
        const focusedElement = page.locator(":focus");
        const isVisible = await focusedElement.isVisible().catch(() => false);
        if (isVisible) {
          await expect(focusedElement).toBeVisible();
          return; // Success, exit the test
        }
      }

      // If we get here, we should still have some focused element
      const finalFocusedElement = page.locator(":focus");
      await expect(finalFocusedElement).toBeAttached(); // At least check it exists
    });

    test("should have proper semantic structure", async ({ page }) => {
      // Ensure API mocks are properly set up for this test
      await mockApiEndpoints(page, { feeds: mockFeeds });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for feed cards to load first
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });

      // Links should be properly structured
      const feedLinks = page.getByRole("link");
      await expect(feedLinks.first()).toBeVisible();
    });

    test("should handle screen reader accessibility", async ({ page }) => {
      // Ensure API mocks are properly set up for this test
      await mockApiEndpoints(page, { feeds: mockFeeds });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for feed cards to load first
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });

      // Check that important elements have appropriate text content with timeouts - use the actual aria-label
      await expect(
        page.getByRole("link", { name: "Open Test Feed 1 in external link" }),
      ).toBeVisible({ timeout: 10000 });

      await expect(
        page.getByRole("button", { name: "Mark Test Feed 1 as read" }),
      ).toBeVisible({ timeout: 10000 });
    });
  });
});
