import { test, expect } from "@playwright/test";

test.describe("DesktopSidebar Filters Mode - TASK1", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to desktop feeds page with filters
    await page.goto("/desktop/feeds");

    // Wait for sidebar to load
    await page.waitForSelector('[data-testid="desktop-sidebar-filters"]', {
      timeout: 5000,
    });
  });
});
