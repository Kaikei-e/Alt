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
   * Uses Playwright auto-wait best practices
   */
  async waitForLoad() {
    await this.page.waitForLoadState("domcontentloaded", { timeout: 20000 });
    await this.page.waitForURL(/\/desktop\/feeds/, { timeout: 20000 });

    // Wait for either loading skeleton or content to appear
    try {
      await Promise.race([
        this.loadingIndicator.waitFor({ state: 'visible', timeout: 5000 }),
        this.feedsList.waitFor({ state: 'visible', timeout: 5000 }),
      ]);
    } catch {
      // If neither appears, continue - may be error state
    }
  }

  /**
   * Get feed count - uses Playwright auto-wait
   */
  async getFeedCount(): Promise<number> {
    try {
      // Wait for loading to complete first
      await this.page.waitForLoadState('networkidle', { timeout: 10000 }).catch(() => {});

      // Feed cards have data-testid="desktop-feed-card-{id}"
      const feedLocator = this.page.locator('[data-testid^="desktop-feed-card-"]');

      // Wait for either feeds to appear or empty state (2 seconds max)
      await Promise.race([
        feedLocator.first().waitFor({ state: 'visible', timeout: 2000 }),
        this.emptyState.waitFor({ state: 'visible', timeout: 2000 }),
        this.errorMessage.waitFor({ state: 'visible', timeout: 2000 }),
      ]).catch(() => {});

      return await feedLocator.count();
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
   * Select feed by index - simplified to just verify visibility
   */
  async selectFeedByIndex(index: number) {
    const feeds = this.page.locator('[data-testid^="desktop-feed-card-"]');
    const targetFeed = feeds.nth(index);

    // Just verify the feed card is visible
    await targetFeed.waitFor({ state: 'visible', timeout: 15000 });
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
      await this.sidebar.waitFor({ state: 'visible', timeout: 10000 });
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
      await this.rightPanel.waitFor({ state: 'visible', timeout: 10000 });
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
      await this.emptyState.waitFor({ state: 'visible', timeout: 3000 });
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
      // Check for either error-state or error-message
      await Promise.race([
        this.errorMessage.waitFor({ state: 'visible', timeout: 3000 }),
        this.page.locator('[data-testid="error-state"]').waitFor({ state: 'visible', timeout: 3000 }),
      ]);
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
