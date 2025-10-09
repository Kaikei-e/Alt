import { Page, Locator, expect } from "@playwright/test";
import { BasePage } from "./BasePage";

/**
 * Desktop Feeds Page Object
 * Represents the /desktop/feeds page
 * Simplified implementation aligned with /tests/pages/ structure
 */
export class DesktopFeedsPage extends BasePage {
  // Locators
  readonly feedsList: Locator;
  readonly addFeedButton: Locator;
  readonly searchInput: Locator;
  readonly sidebar: Locator;
  readonly rightPanel: Locator;
  readonly emptyState: Locator;
  readonly errorMessage: Locator;
  readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators using ACTUAL data-testid from DesktopTimeline.tsx
    // Line 878: desktop-timeline-container is the main scrollable container
    this.feedsList = page.locator('[data-testid="desktop-timeline-container"]');

    // These are in DesktopLayout, not in DesktopTimeline
    // Use role-based selectors as fallback
    this.addFeedButton = page
      .getByRole("link", { name: /register/i })
      .or(page.getByRole("button", { name: /add|register|new feed|\+/i }));
    this.searchInput = page
      .getByRole("link", { name: /search/i })
      .or(page.getByRole("searchbox"))
      .or(page.getByPlaceholder(/search/i));

    // These testids are in DesktopLayout component, not DesktopTimeline
    // Will be visible if DesktopLayout is rendered
    this.sidebar = page.locator('[data-testid="desktop-navigation"]');
    this.rightPanel = page.locator('[data-testid="right-panel"]');

    // Line 990: empty-state appears when visibleFeeds.length === 0
    this.emptyState = page.locator('[data-testid="empty-state"]');

    // Line 839: error-message inside error-state
    this.errorMessage = page.locator('[data-testid="error-message"]');

    // Line 805: skeleton shown during isInitialLoading
    this.loadingIndicator = page.locator(
      '[data-testid="desktop-timeline-skeleton"]',
    );
  }

  /**
   * Navigate to feeds page
   */
  async navigateToFeeds() {
    await this.goto("/desktop/feeds");
    await this.page.waitForLoadState("domcontentloaded", { timeout: 10000 });
  }

  /**
   * Wait for page to be fully loaded
   * Simplified: Just wait for DOM and URL
   */
  async waitForLoad() {
    await this.page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await this.page.waitForURL(/\/desktop\/feeds/, { timeout: 10000 });
    await this.page.waitForTimeout(500);
  }

  /**
   * Get feed count - returns 0 if no feeds or error
   */
  async getFeedCount(): Promise<number> {
    try {
      // Wait a bit for feeds to potentially load
      await this.page.waitForTimeout(1000);

      // Feed cards have data-testid="desktop-feed-card-{id}"
      const items = await this.page
        .locator('[data-testid^="desktop-feed-card-"]')
        .count();

      // If no feed cards, check if we're in loading or error state
      if (items === 0) {
        const isLoading = await this.loadingIndicator
          .isVisible()
          .catch(() => false);
        const hasError = await this.errorMessage.isVisible().catch(() => false);

        if (!isLoading && !hasError) {
          // Wait a bit more in case feeds are still loading
          await this.page.waitForTimeout(2000);
          return await this.page
            .locator('[data-testid^="desktop-feed-card-"]')
            .count();
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
  async clickAddFeed() {
    await this.page.goto("/desktop/feeds/register");
    await this.page.waitForURL(/\/desktop\/feeds\/register/);
  }

  /**
   * Search for feeds (navigate to search page)
   */
  async searchFeed(query: string) {
    await this.searchInput.click();
    await this.page.waitForURL(/\/desktop\/articles\/search/);
  }

  /**
   * Select a feed by title
   */
  async selectFeed(feedTitle: string) {
    const feed = this.page
      .locator('[data-testid^="desktop-feed-card-"]')
      .filter({ hasText: feedTitle });
    await feed.click();
  }

  /**
   * Select feed by index
   */
  async selectFeedByIndex(index: number) {
    const feeds = this.page.locator('[data-testid^="desktop-feed-card-"]');
    const targetFeed = feeds.nth(index);

    await expect(targetFeed).toBeVisible({ timeout: 10000 });
    await targetFeed.click({ timeout: 10000 });
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
   * Mark feed as favorite
   */
  async markAsFavorite(feedTitle: string) {
    const feed = this.page
      .locator('[data-testid^="desktop-feed-card-"]')
      .filter({ hasText: feedTitle });

    const favoriteButton = feed.getByRole("button", {
      name: /favorite|star|いいね/i,
    });
    await favoriteButton.click();
  }

  /**
   * Delete feed by title
   */
  async deleteFeed(feedTitle: string) {
    const feed = this.page
      .locator('[data-testid^="desktop-feed-card-"]')
      .filter({ hasText: feedTitle });

    const deleteButton = feed.getByRole("button", { name: /delete|remove/i });
    await deleteButton.click();

    // Confirm deletion if dialog appears
    const confirmButton = this.page.getByRole("button", {
      name: /confirm|yes|delete/i,
    });

    if ((await confirmButton.count()) > 0) {
      await confirmButton.click();
    }
  }
}
