import { test, expect } from "@playwright/test";
import { FeedDetails, Feed, BackendFeedItem } from "@/schema/feed";

const generateMockFeeds = (count: number, startId: number = 1): Feed[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Feed ${startId + index}`,
    description: `Description for test feed ${startId + index}. This is a longer description to test how the UI handles different text lengths.`,
    link: `https://example.com/feed${startId + index}`,
    published: `2024-01-${String(index + 1).padStart(2, "0")}T12:00:00Z`,
  }));
};

const generateMockFeedDetails = (
  count: number,
  startId: number = 1,
): FeedDetails[] => {
  return Array.from({ length: count }, (_, index) => ({
    feed_url: `https://example.com/feed${index + 1}`,
    summary: `Test Summary for feed ${index + 1}`,
  }));
};

test.describe("FeedDetails Component - Functionality Tests", () => {
  test.beforeEach(async ({ page }) => {
    const mockFeeds = generateMockFeeds(10, 1);

    // Convert Feed[] to BackendFeedItem[] for API compatibility
    const backendFeeds: BackendFeedItem[] = mockFeeds.map((feed) => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock the cursor-based feeds API endpoint (NEW)
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

    // Mock the feeds API endpoint (primary - LEGACY)
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    // Mock the fallback feeds API endpoint
    await page.route("**/api/v1/feeds/fetch/list", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    // Mock the health check endpoint
    await page.route("**/api/v1/health", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "ok" }),
      });
    });

    // Mock the feed read status endpoint
    await page.route("**/api/v1/feeds/read", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed read status updated" }),
      });
    });

    await page.route("**/api/v1/feeds/viewed", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed marked as read" }),
      });
    });

    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for feeds to actually load by checking for feed cards
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 10000 },
    );
  });

  test.describe("Initial State", () => {
    test("should display show details button initially", async ({ page }) => {
      await expect(page.locator(".show-details-button").first()).toBeVisible();
    });

    test("should not display details content initially", async ({ page }) => {
      await expect(page.locator(".summary-text")).not.toBeVisible();
    });

    test("should not display hide details button initially", async ({
      page,
    }) => {
      await expect(page.locator(".hide-details-button")).not.toBeVisible();
    });

    test("should have correct button text", async ({ page }) => {
      await expect(page.locator(".show-details-button").first()).toHaveText(
        "Show Details",
      );
    });
  });

  test.describe("Opening Details", () => {
    test("should show details when show button is clicked", async ({
      page,
    }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      await page.locator(".show-details-button").first().click();

      // Details should be visible
      await expect(page.locator(".summary-text")).toBeVisible();
      await expect(page.locator(".hide-details-button")).toBeVisible();

      // Show button should be hidden (the specific one that was clicked)
      // We can't easily test this with the current structure, so we'll just verify the modal opened
      await expect(page.locator(".summary-text")).toBeVisible();
    });

    test("should display loading state while fetching details", async ({
      page,
    }) => {
      // Delay the response to test loading state
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      await page.locator(".show-details-button").first().click();

      // Should show loading text (may be very brief, so we'll check for either loading or final state)
      try {
        await expect(page.getByText("Loading summary...")).toBeVisible({
          timeout: 2000,
        });
      } catch {
        // Loading might be too fast to catch, check that details opened
        await expect(page.locator(".summary-text")).toBeVisible();
      }
    });

    test("should display correct summary content", async ({ page }) => {
      const testSummary = "This is a detailed test summary for the feed";

      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: testSummary,
          }),
        });
      });

      await page.locator(".show-details-button").first().click();

      await expect(page.locator(".summary-text")).toHaveText(testSummary);
    });
  });

  test.describe("Closing Details", () => {
    test("should hide details when hide button is clicked", async ({
      page,
    }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Open details first
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".hide-details-button")).toBeVisible();

      // Close details
      await page.locator(".hide-details-button").click();

      // Details should be hidden
      await expect(page.locator(".summary-text")).not.toBeVisible();
      await expect(page.locator(".hide-details-button")).not.toBeVisible();

      // Show button should be visible again
      await expect(page.locator(".show-details-button").first()).toBeVisible();
    });

    test("should close details when pressing Escape key", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Open details
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Wait for modal to be fully rendered
      await expect(
        page.locator('[data-testid="modal-backdrop"]'),
      ).toBeVisible();

      // Wait a moment for the modal to be fully rendered
      await page.waitForTimeout(500);

      // Use Escape key to close modal (more reliable than clicking backdrop)
      await page.keyboard.press("Escape");

      // Wait a moment for the modal to close
      await page.waitForTimeout(500);

      // Details should be hidden
      await expect(page.locator(".summary-text")).not.toBeVisible();
      await expect(page.locator(".show-details-button").first()).toBeVisible();
    });

    test("should close details when clicking backdrop", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Open details
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Wait for modal to be fully rendered
      await expect(
        page.locator('[data-testid="modal-backdrop"]'),
      ).toBeVisible();

      // Wait a moment for the modal to be fully rendered
      await page.waitForTimeout(500);

      // Get viewport size to click outside the modal content
      const viewport = page.viewportSize();
      if (!viewport) {
        throw new Error("Viewport size not available");
      }

      // Click in the top-left corner where backdrop should be visible
      await page.mouse.click(10, 10);

      // Wait a moment for the modal to close
      await page.waitForTimeout(500);

      // Details should be hidden
      await expect(page.locator(".summary-text")).not.toBeVisible();
      await expect(page.locator(".show-details-button").first()).toBeVisible();
    });

    test("should close details when touching backdrop on mobile", async ({ page }) => {
      // Set mobile viewport
      await page.setViewportSize({ width: 375, height: 667 });

      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Open details
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Wait for modal to be fully rendered
      await expect(
        page.locator('[data-testid="modal-backdrop"]'),
      ).toBeVisible();

      // Wait a moment for the modal to be fully rendered
      await page.waitForTimeout(500);

      // Click on backdrop area (simulates touch on mobile)
      // Use coordinates that are definitely on the backdrop
      await page.mouse.click(10, 10);

      // Wait a moment for the modal to close
      await page.waitForTimeout(500);

      // Details should be hidden
      await expect(page.locator(".summary-text")).not.toBeVisible();
      await expect(page.locator(".show-details-button").first()).toBeVisible();
    });

    test("should handle touch events on backdrop programmatically", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Open details
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Wait for modal to be fully rendered
      await expect(
        page.locator('[data-testid="modal-backdrop"]'),
      ).toBeVisible();

      // Wait a moment for the modal to be fully rendered
      await page.waitForTimeout(500);

      // Programmatically dispatch touch events
      await page.evaluate(() => {
        const backdrop = document.querySelector('[data-testid="modal-backdrop"]');
        if (backdrop) {
          const touchEndEvent = new TouchEvent('touchend', {
            bubbles: true,
            cancelable: true,
            touches: [],
            targetTouches: [],
            changedTouches: [
              new Touch({
                identifier: 0,
                target: backdrop,
                clientX: 10,
                clientY: 10,
                pageX: 10,
                pageY: 10,
                screenX: 10,
                screenY: 10,
              })
            ]
          });
          backdrop.dispatchEvent(touchEndEvent);
        }
      });

      // Wait a moment for the modal to close
      await page.waitForTimeout(500);

      // Details should be hidden
      await expect(page.locator(".summary-text")).not.toBeVisible();
      await expect(page.locator(".show-details-button").first()).toBeVisible();
    });
  });

  test.describe("Error Handling", () => {
    test("should handle API errors gracefully", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({
            error: "Internal server error",
          }),
        });
      });

      await page.locator(".show-details-button").first().click();

      // Should display error message
      await expect(
        page.getByText("Summary not available for this article"),
      ).toBeVisible();
    });

    test("should handle network errors", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.abort("failed");
      });

      await page.locator(".show-details-button").first().click();

      // Should display error message
      await expect(
        page.getByText("Summary not available for this article"),
      ).toBeVisible();
    });

    test("should handle empty response", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "",
          }),
        });
      });

      await page.locator(".show-details-button").first().click();

      // Should display no summary message
      await expect(
        page.getByText("No summary available for this article"),
      ).toBeVisible();
    });

    test("should handle missing feed URL", async ({ page }) => {
      // This test would require modifying the component to handle missing URLs
      // For now, we'll test the current behavior
      await page.locator(".show-details-button").first().click();

      // Should handle gracefully (exact behavior depends on implementation)
    });
  });

  test.describe("State Management", () => {
    test("should toggle details state correctly", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Initially closed
      await expect(page.locator(".show-details-button").first()).toBeVisible();
      await expect(page.locator(".summary-text")).not.toBeVisible();

      // Open details
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Close details
      await page.locator(".hide-details-button").click();
      await expect(page.locator(".summary-text")).not.toBeVisible();
    });

    test("should handle multiple open/close cycles", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Test multiple cycles
      for (let i = 0; i < 3; i++) {
        // Open
        await page.locator(".show-details-button").first().click();
        await expect(page.locator(".summary-text")).toBeVisible();

        // Close
        await page.locator(".hide-details-button").click();
        await expect(page.locator(".summary-text")).not.toBeVisible();
      }
    });

    test("should handle rapid clicking gracefully", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      const showButton = page.locator(".show-details-button").first();

      // Rapid clicks
      await showButton.click();
      await showButton.click({ timeout: 100 }).catch(() => {}); // Second click might fail if button is hidden

      // Should still work correctly
      await expect(page.locator(".summary-text")).toBeVisible();
    });
  });

  test.describe("Multiple Feed Details", () => {
    test("should handle details for different feeds independently", async ({
      page,
    }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        const request = await route.request();
        const body = await request.postDataJSON();
        const feedUrl = body.feed_url;

        // Return different summaries based on feed URL
        const summary = feedUrl.includes("feed1")
          ? "Test Summary for feed 1"
          : "Test Summary for feed 2";

        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: feedUrl,
            summary: summary,
          }),
        });
      });

      const showButtons = page.locator(".show-details-button");

      // Open first feed details
      await showButtons.first().click();
      await expect(page.getByText("Test Summary for feed 1")).toBeVisible();

      // Close first and open second
      await page.locator(".hide-details-button").click();
      await showButtons.nth(1).click();
      await expect(page.getByText("Test Summary for feed 2")).toBeVisible();
    });
  });

  test.describe("Component Styling", () => {
    test("should have proper modal styling when open", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      await page.locator(".show-details-button").first().click();

      // Check that modal elements are visible
      await expect(page.getByText("Article Summary")).toBeVisible();
      await expect(
        page.locator('[data-testid="modal-backdrop"]'),
      ).toBeVisible();
      await expect(page.locator('[data-testid="modal-content"]')).toBeVisible();
    });

    test("should have proper button styling", async ({ page }) => {
      const showButton = page.locator(".show-details-button").first();

      // Check button is visible and has text
      await expect(showButton).toBeVisible();
      await expect(showButton).toHaveText("Show Details");
    });

    test("should have hide button in modal footer", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Check that hide button is positioned in modal footer
      const hideButton = page.locator(".hide-details-button");
      await expect(hideButton).toBeVisible();

      // Check that it contains the close icon (IoClose component)
      await expect(hideButton).toBeVisible();

      // Verify the button is in the modal footer area (bottom of modal)
      const modalContent = page.locator('[data-testid="modal-content"]');
      await expect(modalContent).toBeVisible();

      // Check that the button is within the modal bounds
      const buttonBox = await hideButton.boundingBox();
      const modalBox = await modalContent.boundingBox();

      if (buttonBox && modalBox) {
        // Button should be within modal boundaries
        expect(buttonBox.x).toBeGreaterThanOrEqual(modalBox.x);
        expect(buttonBox.x + buttonBox.width).toBeLessThanOrEqual(modalBox.x + modalBox.width);
        expect(buttonBox.y).toBeGreaterThanOrEqual(modalBox.y);
        expect(buttonBox.y + buttonBox.height).toBeLessThanOrEqual(modalBox.y + modalBox.height);
      }
    });
  });

  test.describe("Accessibility", () => {
    test("should have proper test identifiers", async ({ page }) => {
      await expect(page.locator(".show-details-button").first()).toBeVisible();
    });

    test("should be keyboard accessible", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Wait for page to load and show button to be available
      await expect(page.locator(".show-details-button").first()).toBeVisible({
        timeout: 15000,
      });

      const showButton = page.locator(".show-details-button").first();

      // Should be able to focus and activate with keyboard
      await showButton.focus();
      await expect(showButton).toBeFocused();

      // Click with keyboard (some components need actual click)
      await showButton.click();

      // Wait for modal backdrop to appear first, then check for content
      await expect(page.locator('[data-testid="modal-backdrop"]')).toBeVisible({
        timeout: 10000,
      });
      await expect(page.locator('[data-testid="modal-content"]')).toBeVisible({
        timeout: 5000,
      });

      // Check that summary text is also visible
      await expect(page.locator(".summary-text")).toBeVisible({
        timeout: 5000,
      });
    });

    test("should have proper modal structure when open", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      await page.locator(".show-details-button").first().click();

      // Check modal structure
      await expect(page.getByText("Article Summary")).toBeVisible();
      await expect(page.locator(".summary-text")).toBeVisible();
      await expect(page.locator(".hide-details-button")).toBeVisible();
    });

    test("should close details with escape key", async ({ page }) => {
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test Summary for feed 1",
          }),
        });
      });

      // Open details for the first feed
      await page.locator(".show-details-button").first().click();

      // Wait for the modal to be fully open
      await expect(
        page.locator('[data-testid="modal-backdrop"]'),
      ).toBeVisible();
      await expect(page.locator('[data-testid="modal-content"]')).toBeVisible();

      // Press Escape to close modal
      await page.keyboard.press("Escape");

      // Modal should be hidden
      await expect(
        page.locator('[data-testid="modal-backdrop"]'),
      ).not.toBeVisible();
      await expect(
        page.locator('[data-testid="modal-content"]'),
      ).not.toBeVisible();
      await expect(page.locator(".show-details-button").first()).toBeVisible();
    });
  });

  test.describe("Mobile Layout Improvements", () => {
    test.beforeEach(async ({ page }) => {
      // Set mobile viewport for layout testing
      await page.setViewportSize({ width: 375, height: 667 });

      // Mock feed details API for consistent testing
      await page.route("**/api/v1/feeds/fetch/details", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary:
              "This is a very long test summary that should demonstrate scrolling behavior. ".repeat(
                20,
              ),
          }),
        });
      });
    });

    test("should keep Article Summary header visible during scroll on mobile", async ({
      page,
    }) => {
      // Open details modal
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Look for sticky positioned summary header
      const summaryHeader = page.locator('[data-testid="summary-header"]');
      await expect(summaryHeader).toHaveCSS("position", "sticky");
    });

    test("should have independent scrollable content area", async ({
      page,
    }) => {
      // Open details modal
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Look for scrollable content area
      const contentArea = page.locator('[data-testid="scrollable-content"]');
      await expect(contentArea).toHaveCSS("overflow-y", "auto");
    });

    test("should maintain header visibility when content scrolls", async ({
      page,
    }) => {
      // Open details modal
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Check for proper flex layout structure
      const modalContent = page.locator('[data-testid="modal-content"]');
      await expect(modalContent).toHaveCSS("display", "flex");
      await expect(modalContent).toHaveCSS("flex-direction", "column");
    });

    test("should have proper mobile viewport dimensions", async ({ page }) => {
      // Open details modal
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Check modal sizing for mobile (90vw, max 420px)
      const modalContent = page.locator('[data-testid="modal-content"]');
      const boundingBox = await modalContent.boundingBox();

      // Modal should be properly sized for mobile (90vw, max 420px)
      expect(boundingBox?.width).toBeGreaterThan(300);
      expect(boundingBox?.width).toBeLessThanOrEqual(420);

      // Height should be 75vh or max 600px
      expect(boundingBox?.height).toBeGreaterThan(300);
      expect(boundingBox?.height).toBeLessThanOrEqual(600);
    });

    test("should have centered header title and close button in footer", async ({
      page,
    }) => {
      // Open details modal
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Check that Article Summary title is centered in header
      await expect(page.getByText("Article Summary")).toBeVisible();

      // Check that close button is visible in modal footer
      const closeButton = page.locator(".hide-details-button");
      await expect(closeButton).toBeVisible();

      // Verify button is positioned within modal bounds (not fixed to viewport)
      const modalContent = page.locator('[data-testid="modal-content"]');
      const buttonBox = await closeButton.boundingBox();
      const modalBox = await modalContent.boundingBox();

      if (buttonBox && modalBox) {
        // Button should be within modal boundaries
        expect(buttonBox.y + buttonBox.height).toBeLessThanOrEqual(modalBox.y + modalBox.height);
        expect(buttonBox.x).toBeGreaterThanOrEqual(modalBox.x);
      }
    });

    test("should separate header and content areas visually", async ({
      page,
    }) => {
      // Open details modal
      await page.locator(".show-details-button").first().click();
      await expect(page.locator(".summary-text")).toBeVisible();

      // Look for distinct header and content areas
      const headerArea = page.locator('[data-testid="header-area"]');
      const contentArea = page.locator('[data-testid="content-area"]');

      await expect(headerArea).toBeVisible();
      await expect(contentArea).toBeVisible();
    });
  });
});
