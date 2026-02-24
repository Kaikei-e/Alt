import { type Locator, type Page, expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Mobile Feed Page Object
 * Uses data-testid selectors for stability
 */
export class MobileFeedPage extends BasePage {
  readonly feedCards: Locator;
  readonly loadingIndicator: Locator;
  readonly emptyState: Locator;
  readonly scrollContainer: Locator;
  readonly markReadButtons: Locator;

  constructor(page: Page) {
    super(page);
    // Use data-testid selectors
    this.feedCards = page.getByTestId("feed-card");
    this.loadingIndicator = page.getByTestId("infinite-scroll-sentinel");
    this.emptyState = page
      .getByTestId("empty-state-icon")
      .or(page.getByTestId("empty-state"));
    this.scrollContainer = page.getByTestId("feeds-scroll-container");
    this.markReadButtons = page.getByRole("button", { name: /mark as read/i });
  }

  /**
   * Navigate to mobile feed page
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
    const markReadButton = card.getByRole("button", { name: /mark as read/i });
    await markReadButton.click();
  }

  /**
   * Get feed card title by index
   */
  async getFeedTitle(index = 0): Promise<string> {
    const card = this.feedCards.nth(index);
    await expect(card).toBeVisible();
    const titleElement = card.locator("a").first();
    return (await titleElement.textContent()) ?? "";
  }

  /**
   * Scroll and load more feeds
   */
  async loadMoreFeeds(): Promise<void> {
    await this.scrollToBottom();
    // Wait for loading indicator
    await expect(this.loadingIndicator)
      .toBeVisible({ timeout: 2000 })
      .catch(() => {});
    await this.page.waitForTimeout(500);
  }
}
