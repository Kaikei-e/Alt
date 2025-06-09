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
  {
    title: "Non relevant feed",
    description: "Non relevant feed",
    link: "https://www.example.com",
    published: "2021-01-01",
    authors: [{ name: "Jane Doe" }],
  },
];

test.describe("SearchWindow", () => {
  test("SearchWindow is functional", async ({ page }) => {
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

    await page.goto("/mobile/search/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for the SearchWindow component to be visible
    await page.waitForSelector("div[data-testid='search-window']", {
      timeout: 10000,
    });

    await page
      .locator("input[data-testid='search-input']")
      .fill("Artificial Intelligence");
    await page.getByRole("button", { name: "Search" }).click();

    await expect(
      page.getByText("Artificial Intelligence is the future"),
    ).toBeVisible();
    await expect(
      page.getByText(
        "Artificial Intelligence and Machine Learning are the future",
      ),
    ).toBeVisible();
    await expect(page.getByText("Non relevant feed")).not.toBeVisible();

    await expect(page.locator("li")).toHaveCount(2);
  });

  test("Check SearchWindow's validation functionality", async ({ page }) => {
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

    await page.goto("/mobile/search/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for the SearchWindow component to be visible
    await page.waitForSelector("div[data-testid='search-window']", {
      timeout: 10000,
    });

    // check the search window works for attaching to the search bar
    // eg. SQL injection
    await page
      .locator("input[data-testid='search-input']")
      .fill("' OR '1' = '1'");
    await page.getByRole("button", { name: "Search" }).click();
    await expect(page.getByText("Enter a valid search query")).toBeVisible();
  });
});
