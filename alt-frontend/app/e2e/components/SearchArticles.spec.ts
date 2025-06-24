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

      // Check that at least some articles are visible with exact matching
      await expect(page.getByText("Test Article 1", { exact: true })).toBeVisible();
    } catch (error) {
      // Log page state for debugging
      console.log("Page content:", await page.content());
      console.log("Expected articles:", mockArticles.length);
      throw error;
    }
  });

  test("should display error message when query is invalid", async ({
    page,
  }) => {
    await page.goto("/mobile/articles/search");
    await page.waitForLoadState("networkidle");

    // Wait for the search form to be available
    await expect(page.locator("[data-testid='search-input']")).toBeVisible({
      timeout: 10000,
    });

    await page.fill("[data-testid='search-input']", "a");

    // Wait for validation error to appear (real-time validation)
    try {
      await page.waitForSelector("[data-testid='error-message']", {
        timeout: 5000,
      });
      const errorMessage = await page.textContent(
        "[data-testid='error-message']",
      );
      expect(errorMessage).toContain(
        "Search query must be at least 2 characters",
      );
    } catch (error) {
      // Real-time validation might not trigger immediately, try submitting
      await page.click("button[type='submit']");
      await page.waitForSelector("[data-testid='error-message']", {
        timeout: 5000,
      });
      const errorMessage = await page.textContent(
        "[data-testid='error-message']",
      );
      expect(errorMessage).toContain(
        "Search query must be at least 2 characters",
      );
    }
  });

  test("should display error message when query is empty", async ({ page }) => {
    await page.goto("/mobile/articles/search");
    await page.waitForLoadState("networkidle");

    // Wait for the search form to be available
    await expect(page.locator("[data-testid='search-input']")).toBeVisible({
      timeout: 10000,
    });

    await page.fill("[data-testid='search-input']", "");
    // Trigger validation by trying to submit
    await page.click("button[type='submit']");

    await page.waitForSelector("[data-testid='error-message']", {
      timeout: 5000,
    });
    const errorMessage = await page.textContent(
      "[data-testid='error-message']",
    );
    expect(errorMessage).toContain("Please enter a search query");
  });
});
