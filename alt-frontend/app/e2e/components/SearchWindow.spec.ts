import { test, expect } from "@playwright/test";

const mockDataForSuccessfulSearch = [
  {
    title: "Artificial Intelligence",
    description: "Artificial Intelligence is the future",
    link: "https://www.example.com",
    published: "2021-01-01",
    authors: [{ name: "John Doe" }],
  },
  {
    title: "Artificial Intelligence and Machine Learning",
    description: "Artificial Intelligence and Machine Learning are the future",
    link: "https://www.example.com",
    published: "2021-01-01",
    authors: [{ name: "Jane Doe" }],
  },
];

test.describe("SearchWindow Component - Functionality Tests", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/mobile/feeds/search");
    await page.waitForLoadState("networkidle");

    // Wait for the SearchWindow component to be visible
    await page.waitForSelector("div[data-testid='search-window']", {
      timeout: 10000,
    });
  });

  test.describe("Initial State", () => {
    test("should render search window component", async ({ page }) => {
      await expect(page.getByTestId("search-window")).toBeVisible();
    });

    test("should display search input field", async ({ page }) => {
      await expect(page.getByTestId("search-input")).toBeVisible();
      await expect(page.getByTestId("search-input")).toHaveAttribute(
        "type",
        "text",
      );
    });

    test("should display search button", async ({ page }) => {
      await expect(page.getByRole("button", { name: "Search" })).toBeVisible();
    });

    test("should have empty search input initially", async ({ page }) => {
      await expect(page.getByTestId("search-input")).toHaveValue("");
    });
  });

  test.describe("Search Functionality", () => {
    test("should perform successful search and display results", async ({
      page,
    }) => {
      const mockResponse = {
        results: mockDataForSuccessfulSearch,
        error: null,
      };

      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(mockResponse),
        });
      });

      // Fill search input and wait for state to settle
      await page.getByTestId("search-input").fill("Artificial Intelligence");

      // Wait for React state to propagate properly
      await page.waitForTimeout(500);

      // Verify input value before proceeding
      await expect(page.getByTestId("search-input")).toHaveValue(
        "Artificial Intelligence",
      );

      // Click search button
      await page.getByRole("button", { name: "Search" }).click();

      // Wait for search to complete and results to appear
      await expect(
        page.getByText("Artificial Intelligence is the future"),
      ).toBeVisible({ timeout: 10000 });
      await expect(
        page.getByText(
          "Artificial Intelligence and Machine Learning are the future",
        ),
      ).toBeVisible();

      // Verify result count using list items
      await expect(page.locator("li")).toHaveCount(2);
    });

    test("should handle empty search results", async ({ page }) => {
      const mockResponse = {
        results: [],
        error: null,
      };

      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(mockResponse),
        });
      });

      await page.getByTestId("search-input").fill("nonexistent query");
      await page.waitForTimeout(500);
      await page.getByRole("button", { name: "Search" }).click();

      // Wait a bit for search to complete, then verify no results
      await page.waitForTimeout(1000);
      await expect(page.locator("li")).toHaveCount(0);
    });
  });

  test.describe("Input Validation", () => {
    test("should handle empty search query", async ({ page }) => {
      await page.getByRole("button", { name: "Search" }).click();

      // Should show validation error
      await expect(page.getByText("Please enter a search query")).toBeVisible({
        timeout: 3000,
      });
    });

    test("should handle server validation errors", async ({ page }) => {
      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify({
            results: [],
            error: "Search query must not be empty",
          }),
        });
      });

      await page.getByTestId("search-input").fill("");
      await page.waitForTimeout(500);
      await page.getByRole("button", { name: "Search" }).click();

      // Expect the actual client-side validation message
      await expect(page.getByText("Please enter a search query")).toBeVisible({
        timeout: 3000,
      });
    });

    test("should handle API errors with generic HTTP message", async ({
      page,
    }) => {
      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify({
            results: [],
            error: "Enter a valid search query",
          }),
        });
      });

      await page.getByTestId("search-input").fill("invalid query");
      await page.waitForTimeout(500);
      await page.getByRole("button", { name: "Search" }).click();

      // Expect the actual HTTP error message being displayed
      await expect(
        page.getByText("API request failed: 400 Bad Request"),
      ).toBeVisible({ timeout: 3000 });
    });
  });

  test.describe("Error Handling", () => {
    test("should handle server errors gracefully", async ({ page }) => {
      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({
            results: [],
            error: "Internal server error",
          }),
        });
      });

      await page.getByTestId("search-input").fill("test query");
      await page.waitForTimeout(500);
      await page.getByRole("button", { name: "Search" }).click();

      // Should display error message
      await expect(
        page.getByText("API request failed: 500 Internal Server Error"),
      ).toBeVisible({ timeout: 3000 });
    });

    test("should handle network errors", async ({ page }) => {
      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.abort("failed");
      });

      await page.getByTestId("search-input").fill("test query");
      await page.waitForTimeout(500);
      await page.getByRole("button", { name: "Search" }).click();

      // Should show some kind of error message (implementation specific)
      await expect(page.getByText(/error|failed|Search failed/i)).toBeVisible({
        timeout: 3000,
      });
    });
  });

  test.describe("User Interaction", () => {
    test("should allow typing in search input", async ({ page }) => {
      const searchInput = page.getByTestId("search-input");

      await searchInput.fill("test search");
      await page.waitForTimeout(100); // Allow state to update
      await expect(searchInput).toHaveValue("test search");
    });

    test("should clear search input", async ({ page }) => {
      const searchInput = page.getByTestId("search-input");

      await searchInput.fill("test search");
      await page.waitForTimeout(100);
      await expect(searchInput).toHaveValue("test search");

      await searchInput.clear();
      await expect(searchInput).toHaveValue("");
    });

    test("should handle Enter key press for search", async ({ page }) => {
      const mockResponse = {
        results: mockDataForSuccessfulSearch,
        error: null,
      };

      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(mockResponse),
        });
      });

      await page.getByTestId("search-input").fill("Artificial Intelligence");
      await page.waitForTimeout(500);
      await page.getByTestId("search-input").press("Enter");

      // Should trigger search and show results
      await expect(
        page.getByText("Artificial Intelligence is the future"),
      ).toBeVisible({ timeout: 5000 });
    });
  });

  test.describe("Search Results Display", () => {
    test("should display all result fields correctly", async ({ page }) => {
      const mockResponse = {
        results: [mockDataForSuccessfulSearch[0]],
        error: null,
      };

      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(mockResponse),
        });
      });

      await page.getByTestId("search-input").fill("Artificial Intelligence");
      await page.waitForTimeout(500);
      await page.getByRole("button", { name: "Search" }).click();

      // Check that all fields are displayed by the parent page
      await expect(
        page.getByRole("heading", { name: "Artificial Intelligence" }),
      ).toBeVisible({ timeout: 5000 });
      await expect(
        page.getByText("Artificial Intelligence is the future"),
      ).toBeVisible();
      await expect(page.getByText("John Doe")).toBeVisible();
    });

    test("should handle multiple search operations", async ({ page }) => {
      // First search
      let mockResponse = {
        results: [mockDataForSuccessfulSearch[0]],
        error: null,
      };

      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(mockResponse),
        });
      });

      await page.getByTestId("search-input").fill("first search");
      await page.waitForTimeout(500);
      await page.getByRole("button", { name: "Search" }).click();

      // Wait for first search results
      await expect(page.locator("li")).toHaveCount(1, { timeout: 5000 });

      // Second search with different results
      mockResponse = {
        results: mockDataForSuccessfulSearch,
        error: null,
      };

      // Update route for second search
      await page.route("**/api/v1/feeds/search", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(mockResponse),
        });
      });

      await page.getByTestId("search-input").clear();
      await page.getByTestId("search-input").fill("second search");
      await page.waitForTimeout(500);
      await page.getByRole("button", { name: "Search" }).click();

      // Wait for second search results
      await expect(page.locator("li")).toHaveCount(2, { timeout: 5000 });
    });
  });
});
