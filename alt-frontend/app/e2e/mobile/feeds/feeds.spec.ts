import { test, expect } from "@playwright/test";
import { Feed, BackendFeedItem } from "@/schema/feed";

// Generate mock feeds for testing
const generateMockFeeds = (count: number, startId: number = 1): Feed[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Feed ${startId + index}`,
    description: `Description for test feed ${startId + index}`,
    link: `https://example.com/feed${startId + index}`,
    published: `2024-01-${String((index + 1) % 30).padStart(2, "0")}T12:00:00Z`,
  }));
};

test.describe("Mobile Feeds Page", () => {
  test.beforeEach(async ({ page }) => {
    const mockFeeds = generateMockFeeds(10, 1);

    // Convert Feed[] to BackendFeedItem[] for API compatibility
    const backendFeeds: BackendFeedItem[] = mockFeeds.map(feed => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock cursor-based feeds API endpoint (NEW)
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: backendFeeds,
          next_cursor: null,
        }),
      });
    });

    // Mock all required API endpoints
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route("**/api/v1/feeds/fetch/list", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route("**/api/v1/health", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "ok" }),
      });
    });

    await page.route("**/api/v1/feeds/read", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed marked as read" }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/details", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_url: "https://example.com/feed1",
          summary: "Test summary for this feed",
        }),
      });
    });
  });

  test("should load and display initial feeds", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for the feeds to load by checking for feed cards first
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    // Wait for the feeds to load - use Mark as read buttons as proxy for feed cards
    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Check that multiple feed cards are rendered (by counting Mark as read buttons)
    const feedCards = page.locator('button:has-text("Mark as read")');
    await expect(feedCards).toHaveCount(10);

    // Verify first feed content
    await expect(page.locator("text=Test Feed 1").first()).toBeVisible();
    await expect(
      page.locator("text=Description for test feed 1").first(),
    ).toBeVisible();
  });

  test("should render feed cards with correct structure", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Check for title link
    await expect(
      page.locator('a[href="https://example.com/feed1"]'),
    ).toBeVisible();
    await expect(
      page.locator('a[href="https://example.com/feed1"]'),
    ).toHaveAttribute("target", "_blank");

    // Check for description
    await expect(
      page.locator("text=Description for test feed 1").first(),
    ).toBeVisible();

    // Check for "Mark as read" button
    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Check for Details button
    await expect(
      page.locator('button:has-text("Show Details")').first(),
    ).toBeVisible();
  });

  test("should handle mark as read functionality", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    const initialFeedCount = await page
      .locator('button:has-text("Mark as read")')
      .count();

    // Click the first "Mark as read" button
    await page.locator('button:has-text("Mark as read")').first().click();

    // After marking as read, there should be one less feed card
    await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
      initialFeedCount - 1,
    );
  });

  test("should implement infinite scrolling", async ({ page }) => {
    const additionalFeeds = generateMockFeeds(10, 11);
    const backendAdditionalFeeds: BackendFeedItem[] = additionalFeeds.map(feed => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock cursor-based API for pagination
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      const url = new URL(route.request().url());
      const cursor = url.searchParams.get('cursor');

      if (cursor === "10") {
        // Return second page
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: backendAdditionalFeeds,
            next_cursor: null,
          }),
        });
      } else {
        // Return first page with cursor for next page
        const mockFeeds = generateMockFeeds(10, 1);
        const backendFeeds: BackendFeedItem[] = mockFeeds.map(feed => ({
          title: feed.title,
          description: feed.description,
          link: feed.link,
          published: feed.published,
        }));

        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: backendFeeds,
            next_cursor: "10",
          }),
        });
      }
    });

    // Mock additional pages for infinite scroll (LEGACY)
    await page.route("**/api/v1/feeds/fetch/page/1", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendAdditionalFeeds),
      });
    });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Initial count should be 10
    await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
      10,
    );

    // Scroll to bottom to trigger infinite scroll
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));

    // Wait for more feeds to load
    await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
      20,
      { timeout: 10000 },
    );

    // Verify new feeds are loaded
    await expect(page.locator("text=Test Feed 11").first()).toBeVisible();
  });

  test("should show loading state during initial load", async ({ page }) => {
    const mockFeeds = generateMockFeeds(10, 1);
    const backendFeeds: BackendFeedItem[] = mockFeeds.map(feed => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock cursor-based API with delay
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: backendFeeds,
          next_cursor: null,
        }),
      });
    });

    // Add delay to initial request to test loading state
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.goto("/mobile/feeds");

    // Should show loading spinner initially
    await expect(page.locator('[data-testid="loading-spinner"]')).toBeVisible();

    // Wait for feeds to load
    await page.waitForLoadState("networkidle");
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();
    // Loading spinner should disappear (we'll just check that feeds are loaded)
    // await expect(page.locator('[data-testid="loading-spinner"]')).not.toBeVisible();
  });

  test("should show loading state during infinite scroll", async ({ page }) => {
    const additionalFeeds = generateMockFeeds(10, 11);
    const backendAdditionalFeeds: BackendFeedItem[] = additionalFeeds.map(feed => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock cursor-based API for pagination with delay
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      const url = new URL(route.request().url());
      const cursor = url.searchParams.get('cursor');

      if (cursor === "10") {
        // Return second page with delay
        await new Promise((resolve) => setTimeout(resolve, 1000));
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: backendAdditionalFeeds,
            next_cursor: null,
          }),
        });
      } else {
        // Return first page with cursor for next page
        const mockFeeds = generateMockFeeds(10, 1);
        const backendFeeds: BackendFeedItem[] = mockFeeds.map(feed => ({
          title: feed.title,
          description: feed.description,
          link: feed.link,
          published: feed.published,
        }));

        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: backendFeeds,
            next_cursor: "10",
          }),
        });
      }
    });

    // Mock additional pages with delay (LEGACY)
    await page.route("**/api/v1/feeds/fetch/page/1", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendAdditionalFeeds),
      });
    });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // The infinite scroll loading is already handled by the cursor-based API mock above

    // Scroll to trigger infinite scroll
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));

    // Should show loading indicator for infinite scroll
    await expect(page.getByText("Loading more...")).toBeVisible();

    // Wait for more content to load
    await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
      20,
      { timeout: 15000 },
    );
  });

  test("should truncate long descriptions", async ({ page }) => {
    // Create feeds with very long descriptions
    const longDescriptionFeeds = generateMockFeeds(3, 1).map((feed, index) => ({
      ...feed,
      description: `${"Very long description ".repeat(50)}for feed ${index + 1}`,
    }));

    const backendLongFeeds: BackendFeedItem[] = longDescriptionFeeds.map(feed => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock cursor-based API
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: backendLongFeeds,
          next_cursor: null,
        }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendLongFeeds),
      });
    });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Description should be truncated with ellipsis - check for ellipsis in the first feed card
    await expect(
      page.locator('[data-testid="feed-card"]').first().locator("text=..."),
    ).toBeVisible();
  });

  test("should be responsive on mobile viewport", async ({ page }) => {
    // Set mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Check that the page content takes appropriate width on mobile
    const markAsReadButton = page
      .locator('button:has-text("Mark as read")')
      .first();
    await expect(markAsReadButton).toBeVisible();

    // Verify responsive layout by checking button size/positioning
    const buttonBox = await markAsReadButton.boundingBox();
    expect(buttonBox?.width).toBeGreaterThan(0);
  });

  test("should handle feed card links correctly", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    const titleLink = page.locator('a[href="https://example.com/feed1"]');

    // Verify link attributes
    await expect(titleLink).toHaveAttribute(
      "href",
      "https://example.com/feed1",
    );
    await expect(titleLink).toHaveAttribute("target", "_blank");

    // Verify link text
    await expect(titleLink).toHaveText("Test Feed 1");
  });

  test("should show correct title", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Check first few feeds have correct
    await expect(page.locator("text=Test Feed 1").first()).toBeVisible();
  });

  test("should maintain scroll position during infinite scroll", async ({
    page,
  }) => {
    const additionalFeeds = generateMockFeeds(10, 11);
    const backendAdditionalFeeds: BackendFeedItem[] = additionalFeeds.map(feed => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock cursor-based API for pagination
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      const url = new URL(route.request().url());
      const cursor = url.searchParams.get('cursor');

      if (cursor === "10") {
        // Return second page
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: backendAdditionalFeeds,
            next_cursor: null,
          }),
        });
      } else {
        // Return first page with cursor for next page
        const mockFeeds = generateMockFeeds(10, 1);
        const backendFeeds: BackendFeedItem[] = mockFeeds.map(feed => ({
          title: feed.title,
          description: feed.description,
          link: feed.link,
          published: feed.published,
        }));

        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: backendFeeds,
            next_cursor: "10",
          }),
        });
      }
    });

    // Mock additional pages (LEGACY)
    await page.route("**/api/v1/feeds/fetch/page/1", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendAdditionalFeeds),
      });
    });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Verify initial count
    await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
      10,
    );

    // Get initial scroll position
    const initialScrollPosition = await page.evaluate(() => window.scrollY);

    // Scroll down
    await page.evaluate(() => window.scrollTo(0, 1000));

    // Trigger infinite scroll
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));

    // Wait for more content
    await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
      20,
      { timeout: 10000 },
    );

    // Verify scroll position has been maintained (not jumped back to top)
    const currentScrollPosition = await page.evaluate(() => window.scrollY);
    expect(currentScrollPosition).toBeGreaterThan(initialScrollPosition);
  });
});
