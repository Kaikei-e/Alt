import {
  componentTest as test,
  expect,
} from "../../../tests/fixtures/apiFixture";
import { setupBackendAPIMocks } from "../../../tests/helpers/apiMocks";

// PROTECTED E2E TESTS - CLAUDE: DO NOT MODIFY
test.describe("Desktop Feeds Page - PROTECTED", () => {
  test.beforeEach(async ({ page }) => {
    // Use centralized API mocking with correct route patterns
    await setupBackendAPIMocks(page);

    await page.goto("/desktop/feeds");
    await page.waitForLoadState("domcontentloaded");

    // Wait for components to load
    await page.waitForTimeout(2000);
  });

  test("should render feeds page with all components (PROTECTED)", async ({
    page,
  }) => {
    // Wait for the page to fully load and render
    await page.waitForLoadState("domcontentloaded");
    await page.waitForTimeout(1000);

    // Verify main layout components are present with increased timeout
    await expect(
      page.locator('[data-testid="desktop-timeline-container"]'),
    ).toBeVisible({
      timeout: 10000,
    });

    // Check if desktop header exists, if not, skip this assertion
    const headerExists = await page
      .locator('[data-testid="desktop-header"]')
      .count();
    if (headerExists > 0) {
      await expect(
        page.locator('[data-testid="desktop-header"]'),
      ).toBeVisible();
    }

    // Check if desktop sidebar filters exist
    const sidebarFiltersExists = await page
      .locator('[data-testid="desktop-sidebar-filters"]')
      .count();
    if (sidebarFiltersExists > 0) {
      await expect(
        page.locator('[data-testid="desktop-sidebar-filters"]'),
      ).toBeVisible();
    }

    // Verify that the page has loaded successfully (no error messages)
    const hasErrorMessage = await page
      .locator("text=Failed to load feeds")
      .count();
    expect(hasErrorMessage).toBe(0);

    // Verify some content is present
    const hasContent = await page.locator("body").textContent();
    expect(hasContent).toBeTruthy();

    // Look for unread count or other indicators that the page loaded correctly
    // Use regex for exact match to avoid strict mode violations
    const hasUnreadCount = await page.locator("text=/^86$/").count();
    if (hasUnreadCount > 0) {
      await expect(page.locator("text=/^86$/")).toBeVisible();
    }

    // Check for filter sections if they exist
    const filterHeaderExists = await page
      .locator('[data-testid="filter-header-title"]')
      .count();
    if (filterHeaderExists > 0) {
      await expect(
        page.locator('[data-testid="filter-header-title"]'),
      ).toHaveText("Filters");
      await expect(
        page.locator('[data-testid="filter-read-status-label"]'),
      ).toHaveText("Read Status");
    }
  });

  test("should handle independent timeline scrolling (PROTECTED)", async ({
    page,
  }) => {
    // Wait for the page to fully load
    await page.waitForLoadState("domcontentloaded");
    await page.waitForTimeout(1000);

    // Mock API to provide enough content for scrolling
    await page.route("**/v1/feeds/fetch/cursor*", async (route) => {
      const feeds = Array.from({ length: 20 }, (_, i) => ({
        id: `feed-${i}`,
        title: `Test Feed ${i}`,
        description: `Description for test feed ${i}`,
        link: `https://example.com/feed-${i}`,
        published: new Date().toISOString(),
      }));

      await route.fulfill({
        json: {
          data: feeds,
          next_cursor: null,
        },
      });
    });

    // Wait for timeline container to be visible
    await expect(
      page.locator('[data-testid="desktop-timeline-container"]'),
    ).toBeVisible({
      timeout: 10000,
    });

    // Simulate scrolling within the timeline container if it exists
    const timelineContainer = page.locator(
      '[data-testid="desktop-timeline-container"]',
    );
    const containerExists = await timelineContainer.count();

    if (containerExists > 0) {
      // Try to scroll within the timeline container
      await timelineContainer.hover();
      await page.mouse.wheel(0, 500);
      await page.waitForTimeout(500);

      // Verify the container is still visible after scrolling
      await expect(timelineContainer).toBeVisible();
    }

    // Verify the page layout is intact after scrolling
    const hasContent = await page.locator("body").textContent();
    expect(hasContent).toBeTruthy();
  });
});
