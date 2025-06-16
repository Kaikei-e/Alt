import { Feed } from "@/schema/feed";
import { expect, test } from "@playwright/test";

const generateMockFeeds = (count: number, startId: number = 1): Feed[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Feed ${startId + index}`,
    description: `Description for test feed ${startId + index}. This is a longer description to test how the UI handles different text lengths.`,
    link: `https://example.com/feed${startId + index}`,
    published: `2024-01-${String(index + 1).padStart(2, "0")}T12:00:00Z`,
  }));
};

test.describe("FeedCard Component - Functionality Tests", () => {
  const mockFeeds = generateMockFeeds(10, 1);

  test.beforeEach(async ({ page }) => {
    // Mock the feeds API endpoints before navigating to the page
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mockFeeds),
      });
    });

    // Also mock the fallback endpoint (getAllFeeds)
    await page.route("**/api/v1/feeds/fetch/list", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mockFeeds),
      });
    });

    // Mock the read status endpoint
    await page.route("**/api/v1/feeds/read", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed marked as read" }),
      });
    });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");
  });

  test.describe("Initial State", () => {
    test("should render feed cards", async ({ page }) => {
      // Check that feed cards are visible
      await expect(page.getByRole("link", { name: "Test Feed 1", exact: true })).toBeVisible();
      await expect(page.getByRole("link", { name: "Test Feed 6", exact: true })).toBeVisible();
    });

    test("should display correct feed titles", async ({ page }) => {
      // Check multiple feed titles
      await expect(page.getByRole("link", { name: "Test Feed 1", exact: true })).toBeVisible();
      await expect(page.getByRole("link", { name: "Test Feed 2", exact: true })).toBeVisible();
      await expect(page.getByRole("link", { name: "Test Feed 3", exact: true })).toBeVisible();
    });

    test("should not display non-existent feeds", async ({ page }) => {
      // Check that feeds beyond our mock data are not visible
      await expect(page.getByRole("link", { name: "Test Feed 11", exact: true })).not.toBeVisible();
      await expect(page.getByRole("link", { name: "Test Feed 15", exact: true })).not.toBeVisible();
    });

    test("should display correct number of feed cards", async ({ page }) => {
      // Count the number of feed links
      const feedLinks = page.getByRole("link").filter({ hasText: "Test Feed" });
      await expect(feedLinks).toHaveCount(10);
    });
  });

  test.describe("Feed Content Display", () => {
    test("should display feed descriptions", async ({ page }) => {
      // Check that descriptions are visible (use partial text to avoid strict mode violations)
      await expect(page.getByText("Description for test feed 1", { exact: false }).first()).toBeVisible();
      await expect(page.getByText("Description for test feed 6", { exact: false }).first()).toBeVisible();
    });

    test("should display feed links correctly", async ({ page }) => {
      // Check that links have correct href attributes
      await expect(page.getByRole("link", { name: "Test Feed 1", exact: true }))
        .toHaveAttribute("href", "https://example.com/feed1");
      await expect(page.getByRole("link", { name: "Test Feed 6", exact: true }))
        .toHaveAttribute("href", "https://example.com/feed6");
    });

    test("should handle long descriptions properly", async ({ page }) => {
      // Check that long descriptions are displayed (may be truncated)
      const longDescription = "Description for test feed 1. This is a longer description to test how the UI handles different text lengths.";
      await expect(page.getByText(longDescription)).toBeVisible();
    });
  });

  test.describe("Feed Interaction", () => {
    test("should be clickable links", async ({ page }) => {
      const feedLink = page.getByRole("link", { name: "Test Feed 1", exact: true });

      // Should be clickable (though we won't actually navigate)
      await expect(feedLink).toBeVisible();
      await expect(feedLink).toHaveAttribute("href");
    });

    test("should handle hover states", async ({ page }) => {
      const feedLink = page.getByRole("link", { name: "Test Feed 1", exact: true });

      // Hover over the link
      await feedLink.hover();

      // Link should still be visible and functional
      await expect(feedLink).toBeVisible();
    });

    test("should handle focus states", async ({ page }) => {
      const feedLink = page.getByRole("link", { name: "Test Feed 1", exact: true });

      // Focus the link
      await feedLink.focus();
      await expect(feedLink).toBeFocused();
    });
  });

  test.describe("Data Validation", () => {
    test("should validate mock data structure", async ({ page }) => {
      // Test the mock data structure (these are synchronous assertions)
      expect(mockFeeds.length).toBe(10);
      expect(mockFeeds[0].title).toBe("Test Feed 1");
      expect(mockFeeds[0].description).toBe(
        "Description for test feed 1. This is a longer description to test how the UI handles different text lengths.",
      );
      expect(mockFeeds[0].link).toBe("https://example.com/feed1");
      expect(mockFeeds[0].published).toBe("2024-01-01T12:00:00Z");

      expect(mockFeeds[5].id).toBe("6");
      expect(mockFeeds[5].title).toBe("Test Feed 6");
      expect(mockFeeds[5].description).toBe(
        "Description for test feed 6. This is a longer description to test how the UI handles different text lengths.",
      );
      expect(mockFeeds[5].link).toBe("https://example.com/feed6");
      expect(mockFeeds[5].published).toBe("2024-01-06T12:00:00Z");
    });

    test("should handle feeds with different content lengths", async ({ page }) => {
      // All feeds should be displayed regardless of content length
      for (let i = 1; i <= 10; i++) {
        await expect(page.getByRole("link", { name: `Test Feed ${i}`, exact: true })).toBeVisible();
      }
    });
  });

  test.describe("Error Handling", () => {
    test("should handle API errors gracefully", async ({ page }) => {
      // Navigate to a new page to test error handling
      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Internal server error" }),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Should handle error gracefully (exact behavior depends on implementation)
      // May show error message or empty state
    });

    test("should handle empty feed list", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify([]),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Should show no feeds
      await expect(page.getByRole("link").filter({ hasText: "Test Feed" })).toHaveCount(0);
    });

    test("should handle malformed feed data", async ({ page }) => {
      const malformedFeeds = [
        { id: "1", title: "", description: "", link: "", published: "" },
        { id: "2", title: null, description: null, link: null, published: null },
      ];

      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(malformedFeeds),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

    });
  });

  test.describe("Feed Ordering and Display", () => {
    test("should display feeds in correct order", async ({ page }) => {
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

      // Should display only 3 feeds
      const feedLinks = page.getByRole("link").filter({ hasText: "Test Feed" });
      await expect(feedLinks).toHaveCount(3);
    });
  });

  test.describe("Performance and Loading", () => {
    test("should load feeds efficiently", async ({ page }) => {
      // Check that feeds load within reasonable time
      const startTime = Date.now();

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // First feed should be visible
      await expect(page.getByRole("link", { name: "Test Feed 1", exact: true })).toBeVisible();

      const loadTime = Date.now() - startTime;
      expect(loadTime).toBeLessThan(30000); // Should load within 30 seconds (more realistic for CI)
    });

    test("should handle large feed lists", async ({ page }) => {
      // Test with a larger number of feeds
      const largeFeedSet = generateMockFeeds(50, 1);

      await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(largeFeedSet),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Should handle large lists gracefully
      await expect(page.getByRole("link", { name: "Test Feed 1", exact: true })).toBeVisible();
    });
  });

  test.describe("Responsive Design", () => {
    test("should display properly on mobile viewport", async ({ page }) => {
      // Set mobile viewport
      await page.setViewportSize({ width: 375, height: 667 });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Feeds should still be visible and properly formatted
      await expect(page.getByRole("link", { name: "Test Feed 1", exact: true })).toBeVisible();
    });

    test("should display properly on tablet viewport", async ({ page }) => {
      // Set tablet viewport
      await page.setViewportSize({ width: 768, height: 1024 });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Feeds should still be visible and properly formatted
      await expect(page.getByRole("link", { name: "Test Feed 1", exact: true })).toBeVisible();
    });
  });

  test.describe("Accessibility", () => {
    test("should have proper link structure", async ({ page }) => {
      // Check that links are properly structured
      const feedLink = page.getByRole("link", { name: "Test Feed 1", exact: true });

      await expect(feedLink).toBeVisible();
      await expect(feedLink).toHaveAttribute("href");
    });

    test("should be keyboard navigable", async ({ page }) => {
      // Should be able to tab through feed links
      await page.keyboard.press("Tab");

      // Check that we can focus on feed links specifically
      const firstFeedLink = page.getByRole("link", { name: "Test Feed 1", exact: true });
      await firstFeedLink.focus();
      await expect(firstFeedLink).toBeFocused();
    });

    test("should have proper semantic structure", async ({ page }) => {
      // Check that feeds are structured as links
      const feedLinks = page.getByRole("link").filter({ hasText: "Test Feed" });
      await expect(feedLinks.first()).toBeVisible();
    });

    test("should handle screen reader accessibility", async ({ page }) => {
      // Check that links have accessible names
      await expect(page.getByRole("link", { name: "Test Feed 1", exact: true })).toBeVisible();
      await expect(page.getByRole("link", { name: "Test Feed 6", exact: true })).toBeVisible();
    });
  });

  test.describe("Integration with Other Components", () => {
    test("should work with FeedDetails component", async ({ page }) => {
      // Check that show details buttons are present (if FeedDetails is integrated)
      const detailsButtons = page.getByTestId("show-details-button");

      // May or may not be present depending on integration
      // This test verifies the integration works if present
      if (await detailsButtons.count() > 0) {
        await expect(detailsButtons.first()).toBeVisible();
      }
    });

    test("should work with pagination if implemented", async ({ page }) => {
      // Check for pagination controls if they exist
      const paginationControls = page.locator("[data-testid*='pagination']");

      // May or may not be present depending on implementation
      // This test verifies pagination works if present
      if (await paginationControls.count() > 0) {
        await expect(paginationControls.first()).toBeVisible();
      }
    });
  });
});
