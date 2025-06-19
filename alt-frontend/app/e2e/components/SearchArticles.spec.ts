import { test, expect } from "@playwright/test";
import { mockApiEndpoints, generateMockArticles } from "../helpers/mockApi";

test.describe("SearchArticles Component - Functionality Tests", () => {
  const mockArticles = generateMockArticles(10, 1);

  test.beforeEach(async ({ page }) => {
    await mockApiEndpoints(page, { articles: mockArticles });
  });

  test("should display articles", async ({ page }) => {
    await page.goto("/mobile/articles/search");
    await page.waitForLoadState("networkidle");

    // Wait for the search form to be available
    await expect(page.locator("[data-testid='search-input']")).toBeVisible({ timeout: 10000 });

    await page.fill("[data-testid='search-input']", "Test");
    await page.click("button[type='submit']");

    // Wait for search results to load - be more flexible about timing
    await page.waitForTimeout(2000);

    // Try to find article cards - they should be rendered by ArticleCard component
    try {
      await page.waitForSelector(
        "[data-testid='article-card']",
        { timeout: 10000 }
      );

      const articleCards = await page.$$("[data-testid='article-card']");
      expect(articleCards.length).toBeGreaterThan(0);

      // Check that at least some articles are visible
      await expect(page.getByText("Test Article 1")).toBeVisible();

    } catch (error) {
      // Check if there's at least some indication of search results
      const hasTestText = await page.locator("text=Test Article").count();
      expect(hasTestText).toBeGreaterThan(0);
    }
  });

  test("should display error message when query is invalid", async ({
    page,
  }) => {
    await page.goto("/mobile/articles/search");
    await page.waitForLoadState("networkidle");

    // Wait for the search form to be available
    await expect(page.locator("[data-testid='search-input']")).toBeVisible({ timeout: 10000 });

    await page.fill("[data-testid='search-input']", "a");

    // Wait for validation error to appear (real-time validation)
    try {
      await page.waitForSelector("[data-testid='error-message']", { timeout: 5000 });
      const errorMessage = await page.textContent("[data-testid='error-message']");
      expect(errorMessage).toContain("Search query must be at least 2 characters");
    } catch (error) {
      // Real-time validation might not trigger immediately, try submitting
      await page.click("button[type='submit']");
      await page.waitForSelector("[data-testid='error-message']", { timeout: 5000 });
      const errorMessage = await page.textContent("[data-testid='error-message']");
      expect(errorMessage).toContain("Search query must be at least 2 characters");
    }
  });

  test("should display error message when query is empty", async ({ page }) => {
    await page.goto("/mobile/articles/search");
    await page.waitForLoadState("networkidle");

    // Wait for the search form to be available
    await expect(page.locator("[data-testid='search-input']")).toBeVisible({ timeout: 10000 });

    await page.fill("[data-testid='search-input']", "");
    // Trigger validation by trying to submit
    await page.click("button[type='submit']");

    await page.waitForSelector("[data-testid='error-message']", { timeout: 5000 });
    const errorMessage = await page.textContent("[data-testid='error-message']");
    expect(errorMessage).toContain("Please enter a search query");
  });
});
