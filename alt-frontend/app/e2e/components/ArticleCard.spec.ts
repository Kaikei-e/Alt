import { Article } from "@/schema/article";
import { expect, test } from "@playwright/test";

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

test.describe("ArticleCard Component - Functionality Tests", () => {
  const mockArticles = generateMockArticles(20, 1);

  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/articles/search", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(mockArticles),
      });
    });
  });

  test("should display articles", async ({ page }) => {
    await page.goto("/mobile/articles/search?q=Test");
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
});
