import { test, expect } from "@playwright/test";

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe("ActivityItem Component - PROTECTED", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a test page that renders the ActivityItem component
    await page.goto("/test/activity-item");
  });

  test("should render with correct icon and content (PROTECTED)", async ({
    page,
  }) => {
    const activityItem = page.locator('[data-testid="activity-item"]');

    await expect(activityItem).toBeVisible();

    // Check title
    const title = activityItem.locator('text="TechCrunch added"');
    await expect(title).toBeVisible();

    // Check time
    const time = activityItem.locator('text="2 min ago"');
    await expect(time).toBeVisible();

    // Check icon is present
    const icon = activityItem.locator("svg");
    await expect(icon).toBeVisible();
  });

  test("should have proper hover effects (PROTECTED)", async ({ page }) => {
    const activityItem = page.locator('[data-testid="activity-item"]');

    // Initial state
    await expect(activityItem).toBeVisible();

    // Hover and check state
    await activityItem.hover();

    // Verify item still visible after hover
    await expect(activityItem).toBeVisible();
  });

  test("should display appropriate icon for activity type (PROTECTED)", async ({
    page,
  }) => {
    const activityItem = page.locator('[data-testid="activity-item"]');
    const iconContainer = activityItem.locator("div").first();

    await expect(iconContainer).toBeVisible();

    // Verify icon container is properly rendered and has content
    const icon = iconContainer.locator("svg");
    await expect(icon).toBeVisible();
  });
});
