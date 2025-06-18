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

test.describe("Search for feeds", () => {
  test("search for feeds", async ({ page }) => {
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

    await page.goto("/mobile/feeds/search");
    await page.waitForLoadState("networkidle");

    // Wait for the SearchWindow component to be visible
    await page.waitForSelector("div[data-testid='search-window']", {
      timeout: 10000,
    });

    // Fill the search input and wait for state to update
    await page.getByTestId("search-input").fill("Artificial Intelligence");
    await page.waitForTimeout(500); // Allow React state to stabilize

    // Verify input value before proceeding
    await expect(page.getByTestId("search-input")).toHaveValue(
      "Artificial Intelligence",
    );

    // Click search button
    await page.getByRole("button", { name: "Search" }).click();

    // Wait for search results to appear
    await expect(
      page.getByText("Artificial Intelligence is the future"),
    ).toBeVisible({ timeout: 5000 });

    await expect(
      page.getByText(
        "Artificial Intelligence and Machine Learning are the future",
      ),
    ).toBeVisible();

    // Verify result count - now looking for proper list items within the results
    await expect(page.locator('[role="list"] li')).toHaveCount(2);
  });

  test("bad search query", async ({ page }) => {
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

    await page.goto("/mobile/feeds/search");
    await page.waitForLoadState("networkidle");

    // Wait for the SearchWindow component to be visible
    await page.waitForSelector("div[data-testid='search-window']", {
      timeout: 10000,
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
