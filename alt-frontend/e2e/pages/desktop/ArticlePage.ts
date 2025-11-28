import { type Locator, type Page, expect } from '@playwright/test';
import { BasePage } from '../BasePage';

export class ArticlePage extends BasePage {
  readonly articleTitle: Locator;
  readonly articleBody: Locator;
  readonly backButton: Locator;
  readonly bookmarkButton: Locator;
  readonly toastNotification: Locator;

  constructor(page: Page) {
    super(page);
    this.articleTitle = page.locator('h1').first();
    this.articleBody = page.locator('[data-testid="article-body"], article, [role="article"]').or(
      page.locator('main article, main [class*="article"], main [class*="content"]')
    );
    this.backButton = page.getByRole('button', { name: /back|戻る|←/i }).or(
      page.locator('a[href*="/desktop/home"], a[href*="/mobile/feeds"]')
    );
    this.bookmarkButton = page.getByRole('button', { name: /bookmark|保存|ブックマーク/i }).or(
      page.locator('[data-testid="bookmark-button"], [aria-label*="bookmark"]')
    );
    this.toastNotification = page.locator('[role="alert"], [data-testid="toast"]').or(
      page.locator('text=/saved|保存しました/i')
    );
  }

  /**
   * Navigate to article detail page
   */
  async goto(articleId: string) {
    await super.goto(`/desktop/articles/${articleId}`);
  }

  /**
   * Wait for article to be fully loaded
   */
  async waitForArticle(timeout = 10000) {
    try {
      await expect(this.articleTitle).toBeVisible({ timeout });
      await expect(this.articleBody).toBeVisible({ timeout });
    } catch (e) {
      const errorText = await this.page.getByText(/Article not found|error occurred/i).textContent().catch(() => null);
      if (errorText) {
        throw new Error(`Article page failed to load: ${errorText}`);
      }
      throw e;
    }
  }

  /**
   * Get article title text
   */
  async getTitle(): Promise<string> {
    return await this.articleTitle.textContent() || '';
  }

  /**
   * Click back button
   */
  async clickBack() {
    await this.backButton.click();
  }

  /**
   * Click bookmark button
   */
  async clickBookmark() {
    await this.bookmarkButton.click();
  }

  /**
   * Wait for toast notification
   */
  async waitForToast(timeout = 5000) {
    await expect(this.toastNotification).toBeVisible({ timeout });
  }
}

