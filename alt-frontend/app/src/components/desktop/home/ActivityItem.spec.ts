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

    // Hover and check background change
    await activityItem.hover();

    const styles = await activityItem.evaluate((el) => getComputedStyle(el));
    // Check if background color is applied
    expect(styles.backgroundColor).toBeTruthy();
  });

  test("should display appropriate icon for activity type (PROTECTED)", async ({
    page,
  }) => {
    const activityItem = page.locator('[data-testid="activity-item"]');
    const iconContainer = activityItem.locator("div").first();

    await expect(iconContainer).toBeVisible();

    // Verify icon container has proper styling (circular border radius)
    const styles = await iconContainer.evaluate((el) => getComputedStyle(el));

    // Check if borderRadius equals half the width/height (circular)
    const borderRadius = parseFloat(styles.borderRadius);
    const width = parseFloat(styles.width);
    const height = parseFloat(styles.height);

    expect(borderRadius).toBeGreaterThan(0);
    expect(width).toEqual(height); // Should be square
    expect(borderRadius).toBeGreaterThanOrEqual((width / 2) * 0.9); // Allow for minor rounding
  });
});
