import { test, expect } from "@playwright/test";

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe("StatsGrid Component - PROTECTED", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a test page that renders the StatsGrid component
    await page.goto("/test/stats-grid");
    await page.waitForLoadState("domcontentloaded");
    await page.waitForTimeout(1000);
  });

  test("should render grid layout with all stats cards (PROTECTED)", async ({
    page,
  }) => {
    const statsGrid = page.locator('[data-testid="stats-grid"]');

    await expect(statsGrid).toBeVisible();

    // Check grid layout properties with Playwright's CSS assertion
    await expect(statsGrid).toHaveCSS("display", "grid");

    // Check all stats cards are present
    const statsCards = statsGrid.locator('[data-testid="stats-card"]');
    await expect(statsCards).toHaveCount(3);

    // Ensure all cards are visible
    for (let i = 0; i < 3; i++) {
      await expect(statsCards.nth(i)).toBeVisible();
    }
  });

  test("should display loading state when isLoading is true (PROTECTED)", async ({
    page,
  }) => {
    await page.goto("/test/stats-grid?loading=true");

    const statsGrid = page.locator('[data-testid="stats-grid"]');
    await expect(statsGrid).toBeVisible();

    // Check loading indicators are present
    const loadingElements = statsGrid.locator('[data-testid="loading"]');
    await expect(loadingElements).toHaveCount(3);
  });

  test("should display error state when error prop is provided (PROTECTED)", async ({
    page,
  }) => {
    await page.goto("/test/stats-grid?error=true");

    const errorMessage = page.locator('[data-testid="error-message"]');
    await expect(errorMessage).toBeVisible();
    await expect(errorMessage).toHaveText("Failed to load statistics");
  });
});
