import { type Locator, type Page, expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Mobile Home Page Object
 * Uses data-testid selectors for stability
 */
export class MobileHomePage extends BasePage {
  readonly feedCards: Locator;
  readonly scrollContainer: Locator;
  readonly emptyState: Locator;
  readonly loadingIndicator: Locator;
  readonly firstFeedCard: Locator;
  readonly bottomNav: Locator;
  readonly markReadButtons: Locator;

  constructor(page: Page) {
    super(page);
    // Use data-testid selectors
    this.feedCards = page.getByTestId("feed-card");
    this.scrollContainer = page.getByTestId("feeds-scroll-container");
    this.emptyState = page
      .getByTestId("empty-state-icon")
      .or(page.getByTestId("empty-state"));
    this.loadingIndicator = page.getByTestId("infinite-scroll-sentinel");
    this.firstFeedCard = this.feedCards.first();
    this.bottomNav = page.locator('nav[role="navigation"]');
    this.markReadButtons = page.getByRole("button", { name: /mark as read/i });
  }

  /**
   * Navigate to mobile home page
   */
  async goto(): Promise<void> {
    await this.navigateTo("/mobile/feeds");
  }

  /**
   * Wait for feeds to load
   */
  async waitForFeeds(timeout = 15000): Promise<void> {
    // Wait for either feed cards or empty state
    await expect(this.feedCards.first().or(this.emptyState)).toBeVisible({
      timeout,
    });
  }

  /**
   * Get number of visible feed cards
   */
  async getFeedCount(): Promise<number> {
    // Wait for cards to render
    await this.page.waitForTimeout(500);
    return this.feedCards.count();
  }

  /**
   * Click on a feed card by index
   */
  async clickFeed(index = 0): Promise<void> {
    const card = this.feedCards.nth(index);
    await expect(card).toBeVisible();
    await card.click();
  }

  /**
   * Mark feed as read by index
   */
  async markAsRead(index = 0): Promise<void> {
    const card = this.feedCards.nth(index);
    await expect(card).toBeVisible();
    const markReadButton = card.getByTestId("mark-as-read-button");
    await markReadButton.click();
  }

  /**
   * Check if empty state is displayed
   */
  async hasEmptyState(): Promise<boolean> {
    try {
      await expect(this.emptyState).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get first feed card title
   */
  async getFirstFeedTitle(): Promise<string> {
    const firstCard = this.feedCards.first();
    await expect(firstCard).toBeVisible();
    // Title is in a link
    const titleElement = firstCard.locator("a").first();
    return (await titleElement.textContent()) ?? "";
  }

  /**
   * Scroll and load more feeds
   */
  async loadMoreFeeds(): Promise<void> {
    await this.scrollToBottom();
    // Wait for loading indicator to appear and disappear
    await expect(this.loadingIndicator)
      .toBeVisible({ timeout: 2000 })
      .catch(() => {});
    await this.page.waitForTimeout(500);
  }

  /**
   * Click on the first feed card
   */
  async clickFirstFeed(): Promise<void> {
    await this.clickFeed(0);
  }

  /**
   * Scroll to load more content
   */
  async scrollToLoadMore(): Promise<void> {
    await this.scrollToBottom();
  }
}
