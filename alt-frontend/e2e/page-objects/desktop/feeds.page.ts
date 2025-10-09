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

    // Initialize locators - using data-testid from DesktopTimeline
    // Use sidebar's "Feeds" link as the heading indicator
    this.pageHeading = page.locator('[data-testid="desktop-navigation"]').getByRole('link', { name: /feeds/i });

    // DesktopTimeline renders feed cards in a container
    // Use the main content area which always exists
    this.feedsList = page.locator('[data-testid="main-content"]');

    // Add feed functionality - navigate to register page via sidebar or direct link
    // The register link is in the sidebar navigation
    this.addFeedButton = page.getByRole('link', { name: /register/i }).or(
      page.getByRole('button', { name: /add|register|new feed|\+|追加/i })
    );

    // Search functionality via sidebar "Search" link
    this.searchInput = page.getByRole('link', { name: /search/i }).or(
      page.getByRole('searchbox')
    ).or(page.getByPlaceholder(/search/i));

    // Sidebar with desktop-navigation testid
    this.sidebar = page.locator('[data-testid="desktop-navigation"]');

    // Right panel with testid
    this.rightPanel = page.locator('[data-testid="right-panel"]');

    // Empty/Error states with testids
    this.emptyState = page.locator('[data-testid="empty-state"]');
    this.errorMessage = page.locator('[data-testid="error-message"]');
    this.loadingIndicator = page.locator('[data-testid="desktop-timeline-skeleton"]');
    this.retryButton = page.locator('[data-testid="retry-button"]');
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
    // Wait for the main layout to be ready
    await this.page.waitForLoadState('domcontentloaded');

    // Wait for sidebar to be visible (always present in desktop layout)
    await expect(this.sidebar).toBeVisible({ timeout: 10000 });

    // Wait for main content area to be visible
    await expect(this.feedsList).toBeVisible({ timeout: 10000 });

    // Wait for either the timeline container or loading/error state
    await this.page.waitForTimeout(2000); // Allow lazy loading and Suspense

    await this.waitForLoadingToComplete();
  }

  /**
   * Get feed count
   */
  async getFeedCount(): Promise<number> {
    try {
      // Wait a bit for feeds to potentially load
      await this.page.waitForTimeout(1000);

      // Feed cards have data-testid="desktop-feed-card-{id}"
      const items = await this.page.locator('[data-testid^="desktop-feed-card-"]').count();

      // If no feed cards, check if we're in loading or error state
      if (items === 0) {
        const isLoading = await this.loadingIndicator.isVisible().catch(() => false);
        const hasError = await this.errorMessage.isVisible().catch(() => false);

        if (!isLoading && !hasError) {
          // Wait a bit more in case feeds are still loading
          await this.page.waitForTimeout(2000);
          return await this.page.locator('[data-testid^="desktop-feed-card-"]').count();
        }
      }

      return items;
    } catch {
      return 0;
    }
  }

  /**
   * Click add feed button (navigate to register page)
   */
  async clickAddFeed(): Promise<void> {
    // Navigate directly to register page since there's no "add" button on this page
    await this.page.goto('/desktop/feeds/register');
    await this.page.waitForURL(/\/desktop\/feeds\/register/);
  }

  /**
   * Search for feeds (navigate to search page)
   */
  async searchFeed(query: string): Promise<void> {
    // Click search link in sidebar to navigate to search page
    await this.searchInput.click();
    await this.page.waitForURL(/\/desktop\/articles\/search/);
  }

  /**
   * Select a feed by title
   */
  async selectFeed(feedTitle: string): Promise<void> {
    const feed = this.page
      .locator('[data-testid^="desktop-feed-card-"]')
      .filter({ hasText: feedTitle });
    await feed.click();
  }

  /**
   * Select feed by index
   */
  async selectFeedByIndex(index: number): Promise<void> {
    const feeds = this.page.locator('[data-testid^="desktop-feed-card-"]');
    await feeds.nth(index).click();
  }

  /**
   * Get all feed titles
   */
  async getFeedTitles(): Promise<string[]> {
    const feeds = this.page.locator('[data-testid^="desktop-feed-card-"]');
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
    const feed = this.page
      .locator('[data-testid^="desktop-feed-card-"]')
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
    const feed = this.page
      .locator('[data-testid^="desktop-feed-card-"]')
      .filter({ hasText: feedTitle });

    const favoriteButton = feed.getByRole('button', { name: /favorite|star|いいね/i });
    await favoriteButton.click();
  }
}
