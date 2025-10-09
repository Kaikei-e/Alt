import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from '../base.page';

/**
 * Desktop Articles Page Object
 * Represents the /desktop/articles page
 */
export class DesktopArticlesPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly articlesList: Locator;
  readonly articleContent: Locator;
  readonly searchLink: Locator;
  readonly filterButton: Locator;
  readonly sortSelect: Locator;
  readonly favoriteIcon: Locator;
  readonly readIcon: Locator;
  readonly sidebar: Locator;
  readonly rightPanel: Locator;
  readonly emptyState: Locator;
  readonly loadingIndicator: Locator;
  readonly paginationNext: Locator;
  readonly paginationPrev: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole('heading', { name: /articles/i });
    this.articlesList = page.getByRole('list').filter({
      has: page.getByRole('article'),
    });
    this.articleContent = page.getByRole('article').first();
    this.searchLink = page.getByRole('link', { name: /search/i });
    this.filterButton = page.getByRole('button', { name: /filter/i });
    this.sortSelect = page.getByLabel(/sort|order/i);
    this.favoriteIcon = page.getByRole('button', { name: /favorite|star/i });
    this.readIcon = page.getByRole('button', { name: /mark.*read/i });
    this.sidebar = page.getByRole('navigation', { name: /sidebar/i });
    this.rightPanel = page.getByRole('complementary');
    this.emptyState = page.getByText(/no articles|empty/i);
    this.loadingIndicator = page.getByRole('status', { name: /loading/i });
    this.paginationNext = page.getByRole('button', { name: /next/i });
    this.paginationPrev = page.getByRole('button', { name: /previous|prev/i });
  }

  /**
   * Navigate to articles page
   */
  async goto(): Promise<void> {
    await this.page.goto('/desktop/articles');
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();

    // Wait for articles or empty state
    try {
      await expect(this.articlesList).toBeVisible({ timeout: 5000 });
    } catch {
      try {
        await expect(this.emptyState).toBeVisible({ timeout: 2000 });
      } catch {
        // Neither visible
      }
    }

    await this.waitForLoadingToComplete();
  }

  /**
   * Get article count
   */
  async getArticleCount(): Promise<number> {
    try {
      return await this.articlesList.getByRole('article').count();
    } catch {
      return 0;
    }
  }

  /**
   * Open article by index
   */
  async openArticle(index: number): Promise<void> {
    const articles = this.articlesList.getByRole('article');
    await articles.nth(index).click();
  }

  /**
   * Open article by title
   */
  async openArticleByTitle(title: string): Promise<void> {
    const article = this.articlesList
      .getByRole('article')
      .filter({ hasText: title });
    await article.click();
  }

  /**
   * Navigate to search
   */
  async goToSearch(): Promise<void> {
    await this.searchLink.click();
    await this.page.waitForURL(/\/desktop\/articles\/search/);
  }

  /**
   * Open filter menu
   */
  async openFilter(): Promise<void> {
    if ((await this.filterButton.count()) > 0) {
      await this.filterButton.click();
    }
  }

  /**
   * Sort articles
   */
  async sortBy(option: string): Promise<void> {
    if ((await this.sortSelect.count()) > 0) {
      await this.sortSelect.selectOption(option);
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Mark article as favorite
   */
  async markAsFavorite(): Promise<void> {
    await this.favoriteIcon.first().click();
  }

  /**
   * Mark article as read
   */
  async markAsRead(): Promise<void> {
    await this.readIcon.first().click();
  }

  /**
   * Go to next page
   */
  async nextPage(): Promise<void> {
    if ((await this.paginationNext.count()) > 0) {
      await this.paginationNext.click();
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Go to previous page
   */
  async previousPage(): Promise<void> {
    if ((await this.paginationPrev.count()) > 0) {
      await this.paginationPrev.click();
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Get all article titles
   */
  async getArticleTitles(): Promise<string[]> {
    const articles = this.articlesList.getByRole('article');
    const headings = articles.getByRole('heading');
    return await headings.allTextContents();
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
   * Filter by feed
   */
  async filterByFeed(feedName: string): Promise<void> {
    await this.openFilter();

    const feedFilter = this.page.getByLabel(/feed/i);
    if ((await feedFilter.count()) > 0) {
      await feedFilter.selectOption(feedName);
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Filter by status (read/unread)
   */
  async filterByStatus(status: 'read' | 'unread' | 'all'): Promise<void> {
    await this.openFilter();

    const statusFilter = this.page.getByLabel(/status/i);
    if ((await statusFilter.count()) > 0) {
      await statusFilter.selectOption(status);
      await this.waitForLoadingToComplete();
    }
  }
}
