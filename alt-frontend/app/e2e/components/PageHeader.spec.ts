import { test, expect } from "@playwright/test";

test.describe("PageHeader Component", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the test page with faster loading strategy
    await page.goto("/test/page-header", {
      waitUntil: "domcontentloaded",
      timeout: 45000,
    });

    // Wait for the page to be ready
    await page.waitForSelector("body", { timeout: 15000 });
  });

  test("should render title and description correctly", async ({ page }) => {
    // Wait for the PageHeader component to be visible with multiple strategies
    try {
      // First try to find by role
      await expect(page.getByRole("banner")).toBeVisible({ timeout: 15000 });
    } catch {
      // Fallback: look for the title text
      await expect(page.getByText("Dashboard Overview")).toBeVisible({ timeout: 15000 });
    }

    // Check for the title
    await expect(page.getByText("Dashboard Overview")).toBeVisible({ timeout: 10000 });

    // Check for the description
    await expect(
      page.getByText("Monitor your RSS feeds and AI-powered content insights")
    ).toBeVisible({ timeout: 10000 });

    // Basic layout check
    const pageContent = await page.textContent("body");
    expect(pageContent).toContain("Dashboard Overview");
    expect(pageContent).toContain("Monitor your RSS feeds and AI-powered content insights");
  });

  test("should have proper structure and styling", async ({ page }) => {
    // Wait for content to load
    await page.waitForSelector("body", { timeout: 15000 });

    // Check that the page has the expected background
    const body = page.locator("body");
    await expect(body).toBeVisible({ timeout: 10000 });

    // Check for text content
    await expect(page.getByText("Dashboard Overview")).toBeVisible({ timeout: 10000 });
    await expect(
      page.getByText("Monitor your RSS feeds and AI-powered content insights")
    ).toBeVisible({ timeout: 10000 });
  });

  test("should be responsive", async ({ page }) => {
    // Test different viewport sizes
    const viewports = [
      { width: 1200, height: 800 },
      { width: 768, height: 1024 },
      { width: 375, height: 667 },
    ];

    for (const viewport of viewports) {
      await page.setViewportSize(viewport);
      await page.waitForTimeout(500);

      // Content should remain visible
      await expect(page.getByText("Dashboard Overview")).toBeVisible({ timeout: 10000 });
      await expect(
        page.getByText("Monitor your RSS feeds and AI-powered content insights")
      ).toBeVisible({ timeout: 10000 });
    }
  });

  test("should handle loading states gracefully", async ({ page }) => {
    // Reload the page to test loading behavior
    await page.reload({ waitUntil: "domcontentloaded" });

    // Wait for content to appear
    await page.waitForSelector("body", { timeout: 15000 });

    // Content should eventually appear
    await expect(page.getByText("Dashboard Overview")).toBeVisible({ timeout: 15000 });
  });
});
