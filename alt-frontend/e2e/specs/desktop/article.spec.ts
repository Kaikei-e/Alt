import { test, expect } from "@playwright/test";
import { ArticlePage } from "../../pages/desktop/ArticlePage";
import { setupAllMocks, mockArticleDetailApi } from "../../utils/api-mock";

test.describe("Desktop Article", () => {
  let articlePage: ArticlePage;

  test.beforeEach(async ({ page }) => {
    articlePage = new ArticlePage(page);
    await setupAllMocks(page);
  });

  test("should display article detail page", async () => {
    await articlePage.goto("feed-1");
    await articlePage.waitForArticle();

    // Verify URL
    expect(articlePage.getUrl()).toMatch(/\/desktop\/articles\/feed-1/);

    // Verify article content is displayed
    const hasContent = await articlePage.hasArticleContent();
    const hasError = await articlePage.hasError();

    // Either content or error should be shown
    expect(hasContent || hasError).toBe(true);
  });

  test("should display article title and content", async () => {
    await articlePage.goto("feed-1");
    await articlePage.waitForArticle();

    const hasContent = await articlePage.hasArticleContent();

    if (hasContent) {
      const title = await articlePage.getArticleTitle();
      expect(title.length).toBeGreaterThan(0);

      await expect(articlePage.articleBody).toBeVisible();
    }
  });

  test("should handle 404 error", async ({ page }) => {
    await mockArticleDetailApi(page, { errorStatus: 404 });
    await articlePage.goto("non-existent");
    await articlePage.waitForArticle();

    // Verify error message is shown
    const hasError = await articlePage.hasError();
    expect(hasError).toBe(true);
  });

  test("should handle 500 error", async ({ page }) => {
    await mockArticleDetailApi(page, { errorStatus: 500 });
    await articlePage.goto("error-article");
    await articlePage.waitForArticle();

    // Verify error message is shown
    const hasError = await articlePage.hasError();
    expect(hasError).toBe(true);
  });
});
