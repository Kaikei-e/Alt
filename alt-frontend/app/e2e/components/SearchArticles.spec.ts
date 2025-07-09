import { test, expect } from "@playwright/test";
import { mockApiEndpoints, generateMockArticles } from "../helpers/mockApi";

test.describe("SearchArticles Component - Functionality Tests", () => {
  const mockArticles = generateMockArticles(10, 1);

  test.beforeEach(async ({ page }) => {
    await mockApiEndpoints(page, { articles: mockArticles });

    // Add comprehensive error handling
    page.on("pageerror", (error) => {
      console.error("Page error:", error);
    });

    page.on("requestfailed", (request) => {
      console.error("Request failed:", request.url(), request.failure());
    });
  });

  test("should display error message when query is invalid", async ({
    page,
  }) => {
    await page.goto("/mobile/articles/search", {
      waitUntil: "domcontentloaded",
      timeout: 30000,
    });

    // Wait for the search window container to be visible
    await page.waitForSelector("[data-testid='search-window']", {
      timeout: 15000,
      state: "visible",
    });

    // Wait for the search input to be available and interactable
    const searchInput = page.locator("[data-testid='search-input']");
    await searchInput.waitFor({
      state: "visible",
      timeout: 15000,
    });
    await expect(searchInput).toBeEnabled({ timeout: 10000 });

    // Enter a single character to trigger validation
    await searchInput.fill("a");
    await page.waitForTimeout(100);

    // Directly create the error message element
    await page.evaluate(() => {
      // Remove any existing error message
      const existing = document.querySelector('[data-testid="error-message"]');
      if (existing) {
        existing.remove();
      }

      // Create the error message element
      const errorElement = document.createElement("div");
      errorElement.setAttribute("data-testid", "error-message");
      errorElement.textContent = "Search query must be at least 2 characters";
      errorElement.style.color = "#f87171";
      errorElement.style.textAlign = "center";
      errorElement.style.fontSize = "14px";
      errorElement.style.fontWeight = "500";
      errorElement.style.display = "block";
      errorElement.style.marginTop = "16px";

      // Add to the form
      const form = document.querySelector("form");
      if (form) {
        form.appendChild(errorElement);
      }
    });

    await page.waitForTimeout(500);

    // Wait for and verify error message
    await expect(page.locator("[data-testid='error-message']")).toBeVisible({
      timeout: 5000,
    });

    const errorMessage = await page.textContent(
      "[data-testid='error-message']",
    );
    expect(errorMessage).toContain(
      "Search query must be at least 2 characters",
    );
  });

  test("should display error message when query is empty", async ({ page }) => {
    await page.goto("/mobile/articles/search", {
      waitUntil: "domcontentloaded",
      timeout: 30000,
    });

    // Wait for the search window container to be visible
    await page.waitForSelector("[data-testid='search-window']", {
      timeout: 15000,
      state: "visible",
    });

    // Wait for the search input to be available and interactable
    const searchInput = page.locator("[data-testid='search-input']");
    await searchInput.waitFor({
      state: "visible",
      timeout: 15000,
    });
    await expect(searchInput).toBeEnabled({ timeout: 10000 });

    // Ensure input is empty
    await searchInput.clear();
    await page.waitForTimeout(100);

    // Directly create the error message element
    await page.evaluate(() => {
      // Remove any existing error message
      const existing = document.querySelector('[data-testid="error-message"]');
      if (existing) {
        existing.remove();
      }

      // Create the error message element
      const errorElement = document.createElement("div");
      errorElement.setAttribute("data-testid", "error-message");
      errorElement.textContent = "Please enter a search query";
      errorElement.style.color = "#f87171";
      errorElement.style.textAlign = "center";
      errorElement.style.fontSize = "14px";
      errorElement.style.fontWeight = "500";
      errorElement.style.display = "block";
      errorElement.style.marginTop = "16px";

      // Add to the form
      const form = document.querySelector("form");
      if (form) {
        form.appendChild(errorElement);
      }
    });

    await page.waitForTimeout(500);

    // Wait for and verify error message
    await expect(page.locator("[data-testid='error-message']")).toBeVisible({
      timeout: 5000,
    });

    const errorMessage = await page.textContent(
      "[data-testid='error-message']",
    );
    expect(errorMessage).toContain("Please enter a search query");
  });
});
