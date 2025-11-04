import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "./BasePage";

/**
 * Desktop Articles Page Object
 * Represents the /desktop/articles page
 * Simplified implementation aligned with /tests/pages/ structure
 */
export class ArticlesPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly articlesTimeline: Locator;
  readonly articlesList: Locator;
  readonly sidebar: Locator;
  readonly rightPanel: Locator;
  readonly searchButton: Locator;
  readonly emptyState: Locator;
  readonly errorMessage: Locator;
  readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators using common patterns
    this.pageHeading = page.locator("h1, h2").first();
    this.articlesTimeline = page.locator('[data-testid="articles-timeline"]');
    this.articlesList = page.locator('[data-testid^="article-"]');
    this.sidebar = page.locator('[data-testid="desktop-navigation"]');
    this.rightPanel = page.locator('[data-testid="right-panel"]');
    this.searchButton = page.getByRole("link", { name: /search/i });
    this.emptyState = page.locator('[data-testid="empty-state"]');
    this.errorMessage = page.locator('[data-testid="error-message"]');
    this.loadingIndicator = page.locator('[data-testid="desktop-timeline-skeleton"]');
  }

  /**
   * Navigate to articles page
   */
  async navigateToArticles() {
    await this.goto("/desktop/articles");
    await this.page.waitForLoadState("domcontentloaded", { timeout: 10000 });
  }

  /**
   * Navigate to articles search page
   */
  async navigateToSearch() {
    await this.goto("/desktop/articles/search");
    await this.page.waitForLoadState("domcontentloaded", { timeout: 10000 });
  }

  /**
   * Wait for page to be fully loaded
   * Simplified: Just wait for DOM and URL
   */
  async waitForLoad() {
    await this.page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await this.page.waitForURL(/\/desktop\/articles/, { timeout: 10000 });
    await this.page.waitForTimeout(500);
  }

  /**
   * Get article count - returns 0 if no articles or error
   */
  async getArticleCount(): Promise<number> {
    try {
      await this.page.waitForTimeout(1000);

      const items = await this.page.locator('[data-testid^="article-"]').count();

      if (items === 0) {
        const isLoading = await this.loadingIndicator.isVisible().catch(() => false);
        const hasError = await this.errorMessage.isVisible().catch(() => false);

        if (!isLoading && !hasError) {
          await this.page.waitForTimeout(2000);
          return await this.page.locator('[data-testid^="article-"]').count();
        }
      }

      return items;
    } catch {
      return 0;
    }
  }

  /**
   * Select an article by title
   */
  async selectArticle(articleTitle: string) {
    const article = this.page
      .locator('[data-testid^="article-"]')
      .filter({ hasText: articleTitle });
    await article.click();
  }

  /**
   * Select article by index
   */
  async selectArticleByIndex(index: number) {
    const articles = this.page.locator('[data-testid^="article-"]');
    const targetArticle = articles.nth(index);

    await expect(targetArticle).toBeVisible({ timeout: 10000 });
    await targetArticle.click({ timeout: 10000 });
  }

  /**
   * Check if sidebar is visible
   */
  async isSidebarVisible(): Promise<boolean> {
    try {
      await expect(this.sidebar).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if right panel is visible
   */
  async isRightPanelVisible(): Promise<boolean> {
    try {
      await expect(this.rightPanel).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if empty state is shown
   */
  async hasEmptyState(): Promise<boolean> {
    try {
      await expect(this.emptyState).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if error is shown
   */
  async hasError(): Promise<boolean> {
    try {
      await expect(this.errorMessage).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }
}
