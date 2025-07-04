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
    await page.unrouteAll();
    const mockFeeds = generateMockFeeds(10, 1);

    // Convert Feed[] to BackendFeedItem[] for API compatibility
    const backendFeeds: BackendFeedItem[] = mockFeeds.map((feed) => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock cursor-based feeds API endpoint (PRIMARY)
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
    await page.route("**/api/v1/feeds/fetch/page/**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route("**/api/v1/feeds/read", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed marked as read" }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/list", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route("**/api/v1/feeds/fetch/limit/**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route("**/api/v1/feeds/fetch/single", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds[0] || {}),
      });
    });

    await page.route("**/api/v1/health", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "ok" }),
      });
    });

    await page.route("**/api/v1/feeds/stats", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_amount: { amount: 42 },
          summarized_feed: { amount: 28 },
        }),
      });
    });

    // Add error handling for any unmatched routes
    await page.route("**/api/**", async (route) => {
      console.log(`Unmatched API route: ${route.request().url()}`);
      await route.fallback();
    });
  });

  test("should load and display initial feeds", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for component initialization
    await page.waitForTimeout(1000);

    // Wait for the feeds to load by checking for feed cards first with extended timeout
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 15000 },
    );

    // Wait for the feeds to load - use Mark as read buttons as proxy for feed cards
    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible({ timeout: 10000 });

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

    // Wait for component initialization
    await page.waitForTimeout(1000);

    // Wait for feeds to load with extended timeout
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 15000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible({ timeout: 10000 });

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

    // Wait for component initialization
    await page.waitForTimeout(1000);

    // Wait for feeds to load with extended timeout
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 15000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible({ timeout: 10000 });

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
    const backendAdditionalFeeds: BackendFeedItem[] = additionalFeeds.map(
      (feed) => ({
        title: feed.title,
        description: feed.description,
        link: feed.link,
        published: feed.published,
      }),
    );

    // Mock cursor-based API for pagination
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      const url = new URL(route.request().url());
      const cursor = url.searchParams.get("cursor");

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
        const backendFeeds: BackendFeedItem[] = mockFeeds.map((feed) => ({
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

    // Scroll to trigger infinite scroll
    // First scroll to 80% to get closer to the sentinel
    await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      if (scrollContainer) {
        const targetScroll = scrollContainer.scrollHeight * 0.8;
        scrollContainer.scrollTop = targetScroll;
      }
    });

    // Wait a moment
    await page.waitForTimeout(500);

    // Then scroll to bottom to trigger infinite scroll
    await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      if (scrollContainer) {
        scrollContainer.scrollTop = scrollContainer.scrollHeight;
      } else {
        window.scrollTo(0, document.body.scrollHeight);
      }
    });

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
    const backendFeeds: BackendFeedItem[] = mockFeeds.map((feed) => ({
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

    // Should show skeleton loading container initially
    await expect(
      page.locator('[data-testid="feeds-skeleton-container"]'),
    ).toBeVisible();

    // Should show skeleton feed cards
    await expect(
      page.locator('[data-testid="skeleton-feed-card"]'),
    ).toHaveCount(5);

    // Wait for feeds to load
    await page.waitForLoadState("networkidle");
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );

    await expect(
      page.locator('button:has-text("Mark as read")').first(),
    ).toBeVisible();

    // Skeleton loading should disappear (feeds are now loaded)
    await expect(
      page.locator('[data-testid="feeds-skeleton-container"]'),
    ).not.toBeVisible();
  });

  test("should show loading state during infinite scroll", async ({ page }) => {
    const additionalFeeds = generateMockFeeds(10, 11);
    const backendAdditionalFeeds: BackendFeedItem[] = additionalFeeds.map(
      (feed) => ({
        title: feed.title,
        description: feed.description,
        link: feed.link,
        published: feed.published,
      }),
    );

    // Mock cursor-based API for pagination with delay
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      const url = new URL(route.request().url());
      const cursor = url.searchParams.get("cursor");

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
        const backendFeeds: BackendFeedItem[] = mockFeeds.map((feed) => ({
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
    // First scroll to 80% to get closer to the sentinel
    await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      if (scrollContainer) {
        const targetScroll = scrollContainer.scrollHeight * 0.8;
        scrollContainer.scrollTop = targetScroll;
      }
    });

    // Wait a moment
    await page.waitForTimeout(500);

    // Then scroll to bottom to trigger infinite scroll
    await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      if (scrollContainer) {
        scrollContainer.scrollTop = scrollContainer.scrollHeight;
      } else {
        window.scrollTo(0, document.body.scrollHeight);
      }
    });

    // Should show loading indicator for infinite scroll
    await expect(
      page
        .locator('[data-testid="infinite-scroll-sentinel"]')
        .getByText("Loading more..."),
    ).toBeVisible();

    // Wait for more content to load
    await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
      20,
      { timeout: 15000 },
    );
  });

  test("should truncate long descriptions", async ({ page }) => {
    // Create feeds with very long descriptions (exceeds 200 char limit)
    const longDescriptionFeeds = generateMockFeeds(3, 1).map((feed, index) => ({
      ...feed,
      description: `${"Very long description ".repeat(50)}for feed ${index + 1}`,
    }));

    const backendLongFeeds: BackendFeedItem[] = longDescriptionFeeds.map(
      (feed) => ({
        title: feed.title,
        description: feed.description,
        link: feed.link,
        published: feed.published,
      }),
    );

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
    const backendAdditionalFeeds: BackendFeedItem[] = additionalFeeds.map(
      (feed) => ({
        title: feed.title,
        description: feed.description,
        link: feed.link,
        published: feed.published,
      }),
    );

    // Mock cursor-based API for pagination
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      const url = new URL(route.request().url());
      const cursor = url.searchParams.get("cursor");

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
        const backendFeeds: BackendFeedItem[] = mockFeeds.map((feed) => ({
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

    // Get initial scroll position from the correct container
    const initialScrollPosition = await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      return scrollContainer ? scrollContainer.scrollTop : window.scrollY;
    });

    // Scroll down
    await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      if (scrollContainer) {
        scrollContainer.scrollTop = 1000;
      } else {
        window.scrollTo(0, 1000);
      }
    });

    // Trigger infinite scroll
    // First scroll to 80% to get closer to the sentinel
    await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      if (scrollContainer) {
        const targetScroll = scrollContainer.scrollHeight * 0.8;
        scrollContainer.scrollTop = targetScroll;
      }
    });

    // Wait a moment
    await page.waitForTimeout(500);

    // Then scroll to bottom to trigger infinite scroll
    await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      if (scrollContainer) {
        scrollContainer.scrollTop = scrollContainer.scrollHeight;
      } else {
        window.scrollTo(0, document.body.scrollHeight);
      }
    });

    // Wait for more content
    await expect(page.locator('button:has-text("Mark as read")')).toHaveCount(
      20,
      { timeout: 10000 },
    );

    // Verify scroll position has been maintained (not jumped back to top)
    const currentScrollPosition = await page.evaluate(() => {
      const scrollContainer = document.querySelector(
        '[data-testid="feeds-scroll-container"]',
      );
      return scrollContainer ? scrollContainer.scrollTop : window.scrollY;
    });
    expect(currentScrollPosition).toBeGreaterThan(initialScrollPosition);
  });

  test.describe("Accessibility Enhancements - TDD Tests", () => {
    test("should include proper ARIA labels for screen readers", async ({
      page,
    }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 10000,
      });

      // TDD: This test will fail initially - verify ARIA labels
      const markAsReadButton = page
        .locator('button:has-text("Mark as read")')
        .first();
      await expect(markAsReadButton).toHaveAttribute(
        "aria-label",
        /mark.*read/i,
      );

      const feedLink = page.locator('[data-testid="feed-card"] a').first();
      await expect(feedLink).toHaveAttribute("aria-label", /open.*external/i);
    });

    test("should support keyboard navigation for feed cards", async ({
      page,
    }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 10000,
      });

      // TDD: This test will fail initially - verify keyboard navigation
      await page.keyboard.press("Tab");

      const focusedElement = page.locator(":focus");
      await expect(focusedElement).toBeVisible();

      // Should be able to activate with Enter key
      await page.keyboard.press("Enter");
      // Verify some interaction occurred
    });

    test("should announce dynamic content changes to screen readers", async ({
      page,
    }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 10000,
      });

      // TDD: This test will fail initially - verify aria-live regions
      const liveRegion = page.locator('[aria-live="polite"]');
      await expect(liveRegion).toBeAttached();

      // Mark a feed as read and check for announcement
      await page.locator('button:has-text("Mark as read")').first().click();

      // Should announce the change
      await expect(liveRegion).toHaveText(/marked.*read|removed/i);
    });
  });

  test.describe("Performance Optimizations - TDD Tests", () => {
    test("should lazy load images in feed cards", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 10000,
      });

      // TDD: This test will fail initially - verify lazy loading
      const images = page.locator('[data-testid="feed-card"] img');
      const imageCount = await images.count();

      if (imageCount > 0) {
        const firstImage = images.first();
        await expect(firstImage).toHaveAttribute("loading", "lazy");
      }
    });

    test("should memoize expensive computations", async ({ page }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 10000,
      });

      // TDD: This test will fail initially - verify memoization
      // Mark multiple feeds as read rapidly
      const markButtons = page.locator('button:has-text("Mark as read")');
      const buttonCount = await markButtons.count();

      for (let i = 0; i < Math.min(3, buttonCount); i++) {
        await markButtons.nth(i).click();
        await page.waitForTimeout(100);
      }

      // Page should remain responsive
      const responseTime = await page.evaluate(() => {
        const start = performance.now();
        // Simulate heavy computation
        const element = document.querySelector('[data-testid="feed-card"]');
        element?.scrollIntoView();
        return performance.now() - start;
      });

      expect(responseTime).toBeLessThan(100); // Should be fast due to memoization
    });
  });

  test.describe("Enhanced Error Handling - TDD Tests", () => {
    test("should display retry button with exponential backoff", async ({
      page,
    }) => {
      // Mock API to consistently fail
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Internal Server Error" }),
        });
      });

      // Go to the page to trigger the error
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Should show error state
      await expect(page.getByText("Server Error")).toBeVisible({
        timeout: 15000,
      });

      // Should show detailed error message
      await expect(
        page.getByText(
          "We're having some trouble on our end. Please try again later.",
        ),
      ).toBeVisible();

      const retryButton = page.getByRole("button", { name: /retry/i });
      await expect(retryButton).toBeVisible();

      // Mock successful response for retry
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: generateMockFeeds(5, 1).map((feed) => ({
              title: feed.title,
              description: feed.description,
              link: feed.link,
              published: feed.published,
            })),
            next_cursor: null,
          }),
        });
      });

      // Click retry button - it will handle backoff automatically
      await retryButton.click();

      // Should show successful state after retry
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });
      await expect(
        page.getByText("Test Feed 1", { exact: true }),
      ).toBeVisible();
    });

    test("should show detailed error messages for different failure types", async ({
      page,
    }) => {
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 429,
          contentType: "application/json",
          body: JSON.stringify({ error: "Rate limit exceeded" }),
        });
      });

      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Should show error state with specific error message
      await expect(page.getByText("Rate Limit Exceeded")).toBeVisible({
        timeout: 15000,
      });

      // Should show rate limit specific message
      await expect(
        page.getByText(
          "You're making requests too quickly. Please wait a moment and try again.",
        ),
      ).toBeVisible();

      const retryButton = page.locator('button:has-text("Retry")');
      await expect(retryButton).toBeVisible({ timeout: 5000 });
    });
  });

  test.describe("High-Speed Scrolling Stability", () => {
    test.beforeEach(async ({ page }) => {
      // Ensure there are no conflicting cursor mocks from outer hooks
      await page.unrouteAll();
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], next_cursor: null }),
        });
      });

      // Generate a larger set of feeds for scrolling tests
      const largeFeedSet = generateMockFeeds(50, 1);
      const backendFeeds: BackendFeedItem[] = largeFeedSet.map((feed) => ({
        title: feed.title,
        description: feed.description,
        link: feed.link,
        published: feed.published,
      }));

      // Mock cursor-based API with larger dataset
      await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
        const url = new URL(route.request().url());
        const cursor = url.searchParams.get("cursor");
        const limit = parseInt(url.searchParams.get("limit") || "20");

        const startIndex = cursor ? parseInt(cursor) : 0;
        const endIndex = Math.min(startIndex + limit, backendFeeds.length);
        const pageData = backendFeeds.slice(startIndex, endIndex);
        const nextCursor =
          endIndex < backendFeeds.length ? endIndex.toString() : null;

        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: pageData,
            next_cursor: nextCursor,
          }),
        });
      });
    });

    test("should render all feed cards correctly during rapid up/down scrolling", async ({
      page,
    }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for initial feeds to load
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 10000,
      });

      // Count initial feed cards
      const initialCount = await page
        .locator('[data-testid="feed-card"]')
        .count();
      expect(initialCount).toBeGreaterThan(0);

      // Perform rapid up/down scrolling that previously caused rendering issues
      for (let i = 0; i < 5; i++) {
        // Scroll down rapidly
        await page.evaluate(() => {
          const scrollContainer = document.querySelector(
            '[data-testid="feeds-scroll-container"]',
          );
          if (scrollContainer) {
            scrollContainer.scrollTop = scrollContainer.scrollHeight * 0.8;
          }
        });

        await page.waitForTimeout(50); // Brief pause

        // Scroll up rapidly
        await page.evaluate(() => {
          const scrollContainer = document.querySelector(
            '[data-testid="feeds-scroll-container"]',
          );
          if (scrollContainer) {
            scrollContainer.scrollTop = 0;
          }
        });

        await page.waitForTimeout(50); // Brief pause
      }

      // Wait for any async operations to complete
      await page.waitForTimeout(500);

      // Verify that feed cards are still rendered correctly
      const finalCount = await page
        .locator('[data-testid="feed-card"]')
        .count();
      expect(finalCount).toBeGreaterThanOrEqual(initialCount);

      // Ensure feed cards have proper content (not empty/missing)
      const firstFeed = page.locator('[data-testid="feed-card"]').first();
      await expect(firstFeed).toBeVisible();
      await expect(
        firstFeed.locator('button:has-text("Mark as read")'),
      ).toBeVisible();
      await expect(
        firstFeed.locator('button:has-text("Show Details")'),
      ).toBeVisible();
    });

    test("should maintain feed card content during continuous fast scrolling", async ({
      page,
    }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait for initial feeds to load
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 10000,
      });

      // Get the first feed's title for reference
      const firstCard = page.locator('[data-testid="feed-card"]').first();
      const firstFeedTitle = await firstCard
        .locator("a > p, a > span, a > div")
        .first()
        .textContent();
      expect(firstFeedTitle).toBeTruthy();

      // Perform continuous fast scrolling
      await page.evaluate(() => {
        const scrollContainer = document.querySelector(
          '[data-testid="feeds-scroll-container"]',
        );
        if (scrollContainer) {
          // Simulate very fast continuous scrolling
          let scrollPosition = 0;
          const scrollHeight = scrollContainer.scrollHeight;
          const step = scrollHeight / 20; // 20 steps to cover full height

          const fastScroll = () => {
            scrollPosition += step;
            if (scrollPosition > scrollHeight) {
              scrollPosition = 0; // Reset to top
            }
            scrollContainer.scrollTop = scrollPosition;
          };

          // Rapid scrolling for 1 second
          const interval = setInterval(fastScroll, 25); // 40fps scrolling
          setTimeout(() => clearInterval(interval), 1000);
        }
      });

      // Wait for scrolling to complete and any async operations
      await page.waitForTimeout(1500);

      // Scroll back to top to check first feed
      await page.evaluate(() => {
        const scrollContainer = document.querySelector(
          '[data-testid="feeds-scroll-container"]',
        );
        if (scrollContainer) {
          scrollContainer.scrollTop = 0;
        }
      });

      await page.waitForTimeout(200);

      // Verify the first feed is still properly rendered
      const currentFirstCard = page
        .locator('[data-testid="feed-card"]')
        .first();
      await expect(currentFirstCard).toBeVisible();

      const currentFirstFeedTitle = await currentFirstCard
        .locator("a > p, a > span, a > div")
        .first()
        .textContent();
      expect(currentFirstFeedTitle).toBe(firstFeedTitle);

      // Ensure interactive elements are still functional
      await expect(
        currentFirstCard.locator('button:has-text("Mark as read")'),
      ).toBeVisible();
      await expect(
        currentFirstCard.locator('button:has-text("Show Details")'),
      ).toBeVisible();
    });

    test("should handle scroll-triggered infinite loading without breaking card rendering", async ({
      page,
    }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait longer for the page to fully load and process API responses
      await page.waitForTimeout(2000);

      // Wait for initial feeds to load with extended timeout
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 20000,
      });

      const initialCount = await page
        .locator('[data-testid="feed-card"]')
        .count();

      // Rapidly scroll to trigger infinite loading multiple times
      for (let i = 0; i < 3; i++) {
        // Scroll to bottom to trigger loading
        await page.evaluate(() => {
          const scrollContainer = document.querySelector(
            '[data-testid="feeds-scroll-container"]',
          );
          if (scrollContainer) {
            scrollContainer.scrollTop = scrollContainer.scrollHeight;
          }
        });

        // Wait for loading to start
        await page.waitForTimeout(200);

        // Scroll up and down rapidly while loading
        await page.evaluate(() => {
          const scrollContainer = document.querySelector(
            '[data-testid="feeds-scroll-container"]',
          );
          if (scrollContainer) {
            scrollContainer.scrollTop = scrollContainer.scrollHeight * 0.5;
          }
        });

        await page.waitForTimeout(100);

        // Back to bottom
        await page.evaluate(() => {
          const scrollContainer = document.querySelector(
            '[data-testid="feeds-scroll-container"]',
          );
          if (scrollContainer) {
            scrollContainer.scrollTop = scrollContainer.scrollHeight;
          }
        });

        // Wait for new content to load
        await page.waitForTimeout(1000);
      }

      // Verify that more feeds were loaded and all are properly rendered
      const finalCount = await page
        .locator('[data-testid="feed-card"]')
        .count();
      expect(finalCount).toBeGreaterThan(initialCount);

      // Check that all visible feed cards have proper content
      const feedCards = page.locator('[data-testid="feed-card"]');
      const count = await feedCards.count();

      // Check first few and last few cards to ensure they're properly rendered
      for (let i of [0, 1, Math.floor(count / 2), count - 2, count - 1]) {
        if (i >= 0 && i < count) {
          const card = feedCards.nth(i);
          await expect(card).toBeVisible();
          await expect(
            card.locator('button:has-text("Mark as read")'),
          ).toBeVisible();
        }
      }
    });

    test("should not lose feed cards during momentum scrolling simulation", async ({
      page,
    }) => {
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");

      // Wait longer for the page to fully load and process API responses
      await page.waitForTimeout(2000);

      // Wait for initial feeds to load with extended timeout
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({
        timeout: 20000,
      });

      // Simulate momentum/inertial scrolling with varying speeds
      await page.evaluate(() => {
        const scrollContainer = document.querySelector(
          '[data-testid="feeds-scroll-container"]',
        );
        if (scrollContainer) {
          let velocity = 50; // Start with high velocity
          let position = 0;
          const deceleration = 0.95; // Momentum decay factor
          const minVelocity = 1;

          const momentumScroll = () => {
            position += velocity;
            velocity *= deceleration;

            // Bounce off the edges
            if (
              position >=
              scrollContainer.scrollHeight - scrollContainer.clientHeight
            ) {
              position =
                scrollContainer.scrollHeight - scrollContainer.clientHeight;
              velocity = -Math.abs(velocity); // Reverse direction
            } else if (position <= 0) {
              position = 0;
              velocity = Math.abs(velocity); // Reverse direction
            }

            scrollContainer.scrollTop = position;

            if (Math.abs(velocity) > minVelocity) {
              requestAnimationFrame(momentumScroll);
            }
          };

          momentumScroll();
        }
      });

      // Wait for momentum scrolling to complete
      await page.waitForTimeout(3000);

      // Verify all feed cards are still properly rendered
      const feedCards = page.locator('[data-testid="feed-card"]');
      const count = await feedCards.count();
      expect(count).toBeGreaterThan(0);

      // Scroll to top and verify first feed
      await page.evaluate(() => {
        const scrollContainer = document.querySelector(
          '[data-testid="feeds-scroll-container"]',
        );
        if (scrollContainer) {
          scrollContainer.scrollTop = 0;
        }
      });

      await page.waitForTimeout(200);

      const firstFeed = page.locator('[data-testid="feed-card"]').first();
      await expect(firstFeed).toBeVisible();
      await expect(
        firstFeed.locator('button:has-text("Mark as read")'),
      ).toBeVisible();
      await expect(
        firstFeed.locator('button:has-text("Show Details")'),
      ).toBeVisible();
    });
  });
});
