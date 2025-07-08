import { test, expect } from "@playwright/test";

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe("QuickActionsPanel Component - PROTECTED", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a test page that renders the QuickActionsPanel component
    await page.goto("/test/quick-actions-panel");
  });

  test("should render with glass effect and action buttons grid (PROTECTED)", async ({
    page,
  }) => {
    const quickActionsPanel = page.locator(
      '[data-testid="quick-actions-panel"]',
    );

    await expect(quickActionsPanel).toBeVisible();

    // Verify glassmorphism visual properties
    const styles = await quickActionsPanel.evaluate((el) =>
      getComputedStyle(el),
    );
    expect(styles.backdropFilter).toContain("blur");
    expect(styles.border).toContain("1px");

    // Check action buttons grid
    const actionsGrid = quickActionsPanel.locator(
      '[data-testid="actions-grid"]',
    );
    await expect(actionsGrid).toBeVisible();

    // Check all action buttons are present
    const actionButtons = actionsGrid.locator('[data-testid="action-button"]');
    await expect(actionButtons).toHaveCount(4);
  });

  test("should display stats footer when additionalStats provided (PROTECTED)", async ({
    page,
  }) => {
    const quickActionsPanel = page.locator(
      '[data-testid="quick-actions-panel"]',
    );

    // Check stats footer is present
    const statsFooter = quickActionsPanel.locator(
      '[data-testid="stats-footer"]',
    );
    await expect(statsFooter).toBeVisible();

    // Check stats content
    await expect(statsFooter).toContainText("Weekly Reads");
    await expect(statsFooter).toContainText("156");
    await expect(statsFooter).toContainText("AI Processed");
    await expect(statsFooter).toContainText("78");
    await expect(statsFooter).toContainText("Bookmarks");
    await expect(statsFooter).toContainText("42");
  });

  test("should not display stats footer when additionalStats not provided (PROTECTED)", async ({
    page,
  }) => {
    await page.goto("/test/quick-actions-panel?noStats=true");

    const quickActionsPanel = page.locator(
      '[data-testid="quick-actions-panel"]',
    );
    const statsFooter = quickActionsPanel.locator(
      '[data-testid="stats-footer"]',
    );

    await expect(statsFooter).not.toBeVisible();
  });
});
