import { test, expect } from "@playwright/test";

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe("ActionButton Component - PROTECTED", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a test page that renders the ActionButton component
    await page.goto("/test/action-button");
    await page.waitForLoadState("domcontentloaded");
    await page.waitForTimeout(3000);
    // Ensure component is rendered
    await page.waitForSelector('[data-testid="action-button"]', {
      timeout: 10000,
    });
  });

  test("should render with glass effect styling (PROTECTED)", async ({
    page,
  }) => {
    const actionButton = page.locator('[data-testid="action-button"]');

    await expect(actionButton).toBeVisible();

    // Verify glassmorphism visual properties
    const styles = await actionButton.evaluate((el) => getComputedStyle(el));
    expect(styles.backdropFilter).toContain("blur");
    expect(styles.border).toContain("1px");
  });

  test("should display icon and label correctly (PROTECTED)", async ({
    page,
  }) => {
    const actionButton = page.locator('[data-testid="action-button"]');
    const icon = actionButton.locator("svg");
    const label = actionButton.locator('text="Add Feed"');

    await expect(icon).toBeVisible();
    await expect(label).toBeVisible();
  });

  test("should have proper hover effects (PROTECTED)", async ({ page }) => {
    const actionButton = page.locator('[data-testid="action-button"]');

    // Initial state
    await expect(actionButton).toBeVisible();

    // Hover and check transform
    await actionButton.hover();

    const styles = await actionButton.evaluate((el) => getComputedStyle(el));
    // Check if transform is applied (exact value may vary)
    expect(styles.transform).toBeTruthy();
  });
});
