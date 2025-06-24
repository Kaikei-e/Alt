import { expect, test } from "@playwright/test";
import { mockApiEndpoints, generateMockArticles } from "../helpers/mockApi";

test.describe("ArticleCard Component - Functionality Tests", () => {
  const mockArticles = generateMockArticles(20, 1);

  test.beforeEach(async ({ page }) => {
    await mockApiEndpoints(page, { articles: mockArticles });
  });

  test("should display articles", async ({ page }) => {
    await page.goto("/mobile/articles/search");
    await page.waitForLoadState("networkidle");

    // Wait for the search form to be available
    await expect(page.locator("[data-testid='search-input']")).toBeVisible({
      timeout: 10000,
    });

    // Fill the input and wait for validation to clear
    await page.fill("[data-testid='search-input']", "Test");

    // Wait for validation to process and button to become enabled
    await expect(page.locator("button[type='submit']")).toBeEnabled({
      timeout: 5000,
    });

    // Ensure input still has the value (validation might have cleared it)
    await expect(page.locator("[data-testid='search-input']")).toHaveValue("Test");

    await page.click("button[type='submit']");

    // Wait for search results to load
    try {
      await page.waitForSelector("[data-testid='article-card']", {
        timeout: 10000,
      });

      const articleCards = await page.$$("[data-testid='article-card']");
      expect(articleCards.length).toBeGreaterThan(0);

      // Check that at least some articles are visible
      await expect(page.getByText("Test Article 1", { exact: true })).toBeVisible();
    } catch (error) {
      // Log page state for debugging
      console.log("Page content:", await page.content());
      throw error;
    }
  });
});
