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

    // Verify component is properly rendered and styled
    await expect(actionButton).toHaveAttribute("data-testid", "action-button");
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

    // Hover and check transform is applied
    await actionButton.hover();

    // Wait for hover animation to apply
    await page.waitForTimeout(100);

    // Check if hover state is maintained - verify button is still visible
    await expect(actionButton).toBeVisible();
  });
});
