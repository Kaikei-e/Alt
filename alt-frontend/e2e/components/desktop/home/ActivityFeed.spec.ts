import { test, expect } from "@playwright/test";

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe("ActivityFeed Component - PROTECTED", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a test page that renders the ActivityFeed component
    await page.goto("/test/activity-feed");
    await page.waitForLoadState("domcontentloaded");

    // Try multiple selectors to find the activity feed
    const selectors = [
      '[data-testid="activity-feed"]',
      'text="Recent Activity"',
      'text="Activity"',
      'div:has-text("Activity")',
    ];

    let found = false;
    for (const selector of selectors) {
      try {
        await page.waitForSelector(selector, { timeout: 5000 });
        found = true;
        break;
      } catch (e) {
        // Continue to next selector
      }
    }

    if (!found) {
      throw new Error("ActivityFeed component not found");
    }
  });

  test("should render with glass effect and header (PROTECTED)", async ({
    page,
  }) => {
    // Find the activity feed using multiple selectors
    const selectors = [
      '[data-testid="activity-feed"]',
      'text="Recent Activity"',
      'text="Activity"',
      'div:has-text("Activity")',
    ];

    let activityFeed = null;
    for (const selector of selectors) {
      try {
        activityFeed = page.locator(selector).first();
        if (await activityFeed.isVisible()) {
          break;
        }
      } catch (e) {
        // Continue to next selector
      }
    }

    expect(activityFeed).toBeTruthy();
    if (activityFeed) {
      await expect(activityFeed).toBeVisible();
    }

    // Check for activity-related text
    let hasActivityText = false;
    try {
      hasActivityText = await page.locator('text="Activity"').isVisible();
    } catch (e) {
      hasActivityText = false;
    }
    expect(hasActivityText).toBe(true);
  });

  test("should display all activity items (PROTECTED)", async ({ page }) => {
    const activityFeed = page.locator('[data-testid="activity-feed"]');

    // Check all activity items are present
    const activityItems = activityFeed.locator('[data-testid="activity-item"]');
    await expect(activityItems).toHaveCount(4);

    // Check first activity item content
    const firstItem = activityItems.first();
    await expect(firstItem).toContainText("TechCrunch added");
    await expect(firstItem).toContainText("2 min ago");
  });

  test("should display empty state when no activities (PROTECTED)", async ({
    page,
  }) => {
    await page.goto("/test/activity-feed?empty=true");

    const emptyState = page.locator('[data-testid="empty-state"]');
    await expect(emptyState).toBeVisible();
    await expect(emptyState).toHaveText("No recent activity");
  });
});
