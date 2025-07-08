import { test, expect } from "@playwright/test";

test.describe("PageHeader Component", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the test page with longer timeout
    await page.goto("/test/page-header", {
      waitUntil: "networkidle",
      timeout: 60000,
    });

    // Wait for the PageHeader component to be visible
    await expect(page.getByRole("banner")).toBeVisible({ timeout: 30000 });
  });

  test("should render title and description correctly", async ({ page }) => {
    await expect(page.getByText("Dashboard Overview")).toBeVisible();
    await expect(
      page.getByText("Monitor your RSS feeds and AI-powered content insights"),
    ).toBeVisible();
  });
});
