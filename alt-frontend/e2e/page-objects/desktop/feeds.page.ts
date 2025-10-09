import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from '../base.page';

/**
 * Desktop Feeds Page Object
 * Represents the /desktop/feeds page
 */
export class DesktopFeedsPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly feedsList: Locator;
  readonly addFeedButton: Locator;
  readonly searchInput: Locator;
  readonly sidebar: Locator;
  readonly rightPanel: Locator;
  readonly emptyState: Locator;
  readonly errorMessage: Locator;
  readonly loadingIndicator: Locator;
  readonly retryButton: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole('heading', { name: /feeds/i });
    this.feedsList = page.getByRole('list').filter({
      has: page.getByRole('article').or(page.getByRole('listitem')),
    });
    this.addFeedButton = page.getByRole('button', { name: /add feed|register|new feed/i });
    this.searchInput = page.getByRole('searchbox').or(
      page.getByPlaceholder(/search/i)
    );
    this.sidebar = page.getByRole('navigation', { name: /sidebar/i });
    this.rightPanel = page.getByRole('complementary', { name: /analytics|stats/i });
    this.emptyState = page.getByText(/no feeds|empty|get started/i);
    this.errorMessage = page.getByRole('alert');
    this.loadingIndicator = page.getByRole('status', { name: /loading/i });
    this.retryButton = page.getByRole('button', { name: /retry|try again/i });
  }

  /**
   * Navigate to feeds page
   */
  async goto(): Promise<void> {
    await this.page.goto('/desktop/feeds');
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();

    // Wait for either feed list or empty state
    try {
      await expect(this.feedsList).toBeVisible({ timeout: 5000 });
    } catch {
      // If no feeds, empty state might be shown
      try {
        await expect(this.emptyState).toBeVisible({ timeout: 2000 });
      } catch {
        // Neither visible, might be loading or error
      }
    }

    await this.waitForLoadingToComplete();
  }

  /**
   * Get feed count
   */
  async getFeedCount(): Promise<number> {
    try {
      const items = await this.feedsList.getByRole('article').count();
      return items;
    } catch {
      return 0;
    }
  }

  /**
   * Click add feed button
   */
  async clickAddFeed(): Promise<void> {
    await this.addFeedButton.click();
    await this.page.waitForURL(/\/desktop\/feeds\/register/);
  }

  /**
   * Search for feeds
   */
  async searchFeed(query: string): Promise<void> {
    await this.searchInput.fill(query);
    await this.searchInput.press('Enter');
  }

  /**
   * Select a feed by title
   */
  async selectFeed(feedTitle: string): Promise<void> {
    const feed = this.feedsList
      .getByRole('article')
      .filter({ hasText: feedTitle });
    await feed.click();
  }

  /**
   * Select feed by index
   */
  async selectFeedByIndex(index: number): Promise<void> {
    const feeds = this.feedsList.getByRole('article');
    await feeds.nth(index).click();
  }

  /**
   * Get all feed titles
   */
  async getFeedTitles(): Promise<string[]> {
    const feeds = this.feedsList.getByRole('article');
    return await feeds.allTextContents();
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

  /**
   * Get error message
   */
  async getError(): Promise<string | null> {
    if (await this.hasError()) {
      return await this.errorMessage.textContent();
    }
    return null;
  }

  /**
   * Click retry button
   */
  async clickRetry(): Promise<void> {
    await this.retryButton.click();
    await this.waitForLoadingToComplete();
  }

  /**
   * Delete feed by title
   */
  async deleteFeed(feedTitle: string): Promise<void> {
    const feed = this.feedsList
      .getByRole('article')
      .filter({ hasText: feedTitle });

    const deleteButton = feed.getByRole('button', { name: /delete|remove/i });
    await deleteButton.click();

    // Confirm deletion if dialog appears
    const confirmButton = this.page.getByRole('button', {
      name: /confirm|yes|delete/i,
    });

    if ((await confirmButton.count()) > 0) {
      await confirmButton.click();
    }
  }

  /**
   * Mark feed as favorite
   */
  async markAsFavorite(feedTitle: string): Promise<void> {
    const feed = this.feedsList
      .getByRole('article')
      .filter({ hasText: feedTitle });

    const favoriteButton = feed.getByRole('button', { name: /favorite|star/i });
    await favoriteButton.click();
  }
}
