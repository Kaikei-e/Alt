import { Article } from "@/schema/article";
import { test, expect } from "@playwright/test";

const generateMockArticles = (
  count: number,
  startId: number = 1,
): Article[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Article ${startId + index}`,
    content: `Content for test article ${startId + index}. This is a longer content to test how the UI handles different text lengths.`,
  }));
};

test.describe("SearchArticles Component - Functionality Tests", () => {
  const mockArticles = generateMockArticles(10, 1);

  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/articles/search?q=Test", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mockArticles),
      });
    });
  });

  test("should display articles", async ({ page }) => {
    await page.goto("/mobile/articles/search");
    await page.fill("input[type='text']", "Test");
    await page.click("button[type='submit']");
    await page.waitForSelector(
      ".article-card-wrapper[data-testid='article-card']",
    );
    const articleCards = await page.$$(
      ".article-card-wrapper[data-testid='article-card']",
    );
    expect(articleCards).toHaveLength(mockArticles.length);

    for (const article of mockArticles) {
      const articleCard = await articleCards.find(
        async (card) => (await card.textContent()) === article.title,
      );
      expect(articleCard).toBeDefined();
    }
  });

  test("should display error message when query is invalid", async ({
    page,
  }) => {
    await page.goto("/mobile/articles/search");
    await page.fill("input[type='text']", "a");
    // Wait for validation error to appear (real-time validation)
    await page.waitForSelector("[data-testid='error-message']");
    const errorMessage = await page.textContent(
      "[data-testid='error-message']",
    );
    expect(errorMessage).toContain(
      "Search query must be at least 2 characters",
    );
  });

  test("should display error message when query is empty", async ({ page }) => {
    await page.goto("/mobile/articles/search");
    await page.fill("input[type='text']", "");
    // Trigger validation by trying to submit or just waiting
    await page.click("button[type='submit']");
    await page.waitForSelector("[data-testid='error-message']");
    const errorMessage = await page.textContent(
      "[data-testid='error-message']",
    );
    expect(errorMessage).toContain("Please enter a search query");
  });
});
