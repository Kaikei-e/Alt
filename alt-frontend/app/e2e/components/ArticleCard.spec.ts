import { expect, test } from "@playwright/test";
import { mockApiEndpoints, generateMockArticles } from "../helpers/mockApi";

test.describe("ArticleCard Component - Functionality Tests", () => {
  const mockArticles = generateMockArticles(20, 1);

  test.beforeEach(async ({ page }) => {
    await mockApiEndpoints(page, { articles: mockArticles });
  });

  test("should display articles", async ({ page }) => {
    await page.goto("/mobile/articles/search?q=Test");

    await page.waitForLoadState("networkidle");

    const pageContent = await page.content();
    console.log("Page loaded, looking for article cards...");

    try {
      await page.waitForSelector("[data-testid='article-card']", {
        timeout: 10000,
      });
    } catch (error) {
      console.log("Failed to find article cards, page content:", pageContent);
      throw error;
    }

    const articleCards = await page.$$("[data-testid='article-card']");
    expect(articleCards).toHaveLength(mockArticles.length);

    for (const article of mockArticles) {
      const articleCard = await articleCards.find(
        async (card) => (await card.textContent()) === article.title,
      );
      expect(articleCard).toBeDefined();
    }
  });
});
