import { test, expect } from "@playwright/test";

test.describe("CallToActionBar Component", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the test page
    await page.goto("/test/call-to-action-bar");
    await page.waitForLoadState("domcontentloaded");
  });

  test("should render title and description correctly", async ({ page }) => {
    await expect(page.getByText("Ready to explore?")).toBeVisible();
    await expect(
      page.getByText("Discover new content and manage your feeds"),
    ).toBeVisible();
  });

  test("should render action buttons correctly", async ({ page }) => {
    const browseButton = page.getByRole("link", { name: "Browse Feeds" });
    const addButton = page.getByRole("link", { name: "Add New Feed" });

    await expect(browseButton).toBeVisible();
    await expect(addButton).toBeVisible();

    // Check button links
    await expect(browseButton).toHaveAttribute("href", "/desktop/feeds");
    await expect(addButton).toHaveAttribute("href", "/desktop/feeds/register");
  });

  test("should have proper responsive layout", async ({ page }) => {
    // Test desktop layout
    await page.setViewportSize({ width: 1200, height: 800 });

    const container = page.locator('[data-testid="cta-container"]');
    await expect(container).toBeVisible();

    // Test tablet layout
    await page.setViewportSize({ width: 768, height: 1024 });
    await expect(container).toBeVisible();

    // Test mobile layout
    await page.setViewportSize({ width: 375, height: 667 });
    await expect(container).toBeVisible();
  });
});
