import { type Locator, type Page, expect } from '@playwright/test';
import { BasePage } from '../BasePage';

/**
 * Desktop Article Detail Page Object
 * Uses semantic and role-based selectors for accessibility
 */
export class ArticlePage extends BasePage {
  readonly articleTitle: Locator;
  readonly articleBody: Locator;
  readonly loadingSpinner: Locator;
  readonly errorMessage: Locator;
  readonly publishedDate: Locator;
  readonly container: Locator;
  readonly articleContainer: Locator;
  readonly articleError: Locator;

  constructor(page: Page) {
    super(page);
    // Use data-testid for stable selection
    this.articleTitle = page.getByTestId('article-title');
    // Article body with .article-content class or data-testid
    this.articleBody = page
      .locator('.article-content')
      .or(page.getByTestId('article-content'));
    // Loading spinner
    this.loadingSpinner = page
      .locator('[class*="spinner"], [class*="loading"]')
      .or(page.getByTestId('article-spinner'))
      .or(page.getByTestId('article-loading'));
    // Error message - multiple patterns
    this.errorMessage = page
      .getByText(/not found|error occurred|見つかりません/i)
      .or(page.getByTestId('article-error-message'));
    // Published date
    this.publishedDate = page.getByText(/published:/i);
    // Container
    this.container = page.locator('main');
    // Article container with data-testid
    this.articleContainer = page.getByTestId('article-container');
    // Article error container
    this.articleError = page.getByTestId('article-error');
  }

  /**
   * Navigate to article detail page
   */
  async goto(articleId: string): Promise<void> {
    await this.navigateTo(`/desktop/articles/${articleId}`);
  }

  /**
   * Wait for article to load
   */
  async waitForArticle(timeout = 15000): Promise<void> {
    // Wait for loading spinner to disappear
    await expect(this.loadingSpinner).toBeHidden({ timeout }).catch(() => {});

    // Wait for either article container or error container
    await expect(
      this.articleContainer.or(this.articleError),
    ).toBeVisible({ timeout });
  }

  /**
   * Get article title text
   */
  async getArticleTitle(): Promise<string> {
    await expect(this.articleTitle).toBeVisible();
    return (await this.articleTitle.textContent()) ?? '';
  }

  /**
   * Check if article loaded successfully
   */
  async hasArticleContent(): Promise<boolean> {
    try {
      await expect(this.articleContainer).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if error is displayed
   */
  async hasError(): Promise<boolean> {
    try {
      // First try data-testid, then fall back to text content
      const errorByTestId = await this.articleError.isVisible();
      if (errorByTestId) return true;

      // Check for error text
      const errorText = this.page.getByText('Article not found or error occurred');
      await expect(errorText).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get article body HTML content
   */
  async getArticleBodyHtml(): Promise<string> {
    await expect(this.articleBody).toBeVisible();
    return (await this.articleBody.innerHTML()) ?? '';
  }

  /**
   * Get article body text content
   */
  async getArticleBodyText(): Promise<string> {
    await expect(this.articleBody).toBeVisible();
    return (await this.articleBody.textContent()) ?? '';
  }

  /**
   * Check if published date is displayed
   */
  async hasPublishedDate(): Promise<boolean> {
    try {
      await expect(this.publishedDate).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
