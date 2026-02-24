import { type Locator, type Page, expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Desktop Feed Page Object
 * Uses data-testid selectors for stability
 */
export class FeedPage extends BasePage {
  // Primary locators using data-testid
  readonly feedCards: Locator;
  readonly emptyState: Locator;
  readonly loadingIndicator: Locator;
  readonly timelineContainer: Locator;
  readonly skeleton: Locator;
  readonly errorState: Locator;
  readonly errorMessage: Locator;
  readonly retryButton: Locator;

  constructor(page: Page) {
    super(page);
    // Use stable data-testid selectors
    this.feedCards = page.locator('[data-testid^="desktop-feed-card-"]');
    this.emptyState = page.getByTestId("empty-state");
    this.loadingIndicator = page.getByTestId("infinite-scroll-sentinel");
    this.timelineContainer = page.getByTestId("desktop-timeline-container");
    this.skeleton = page.getByTestId("desktop-timeline-skeleton");
    this.errorState = page.getByTestId("error-state");
    this.errorMessage = page.getByTestId("error-message");
    this.retryButton = page.getByTestId("retry-button");
  }

  /**
   * Navigate to feed page
   */
  async goto(): Promise<void> {
    await this.navigateTo("/desktop/feeds");
  }

  /**
   * Wait for feeds to load and be visible
   */
  async waitForFeeds(timeout = 15000): Promise<void> {
    // Wait for skeleton to disappear (if visible)
    await expect(this.skeleton)
      .toBeHidden({ timeout })
      .catch(() => {});

    // Wait for either feed cards, empty state, or error state
    await expect(
      this.feedCards.first().or(this.emptyState).or(this.errorState),
    ).toBeVisible({ timeout });
  }

  /**
   * Get number of visible feed cards
   */
  async getFeedCount(): Promise<number> {
    // Wait a bit for cards to render
    await this.page.waitForTimeout(500);
    return this.feedCards.count();
  }

  /**
   * Get first feed card's title
   */
  async getFirstFeedTitle(): Promise<string> {
    const firstCard = this.feedCards.first();
    await expect(firstCard).toBeVisible();
    // Use data-testid for stable selection
    const titleElement = firstCard.getByTestId("feed-card-title");
    return (await titleElement.textContent()) ?? "";
  }

  /**
   * Click on a specific feed card by index
   */
  async clickFeed(index = 0): Promise<void> {
    const card = this.feedCards.nth(index);
    await expect(card).toBeVisible();
    await card.click();
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
   * Check if error state is displayed
   */
  async hasErrorState(): Promise<boolean> {
    try {
      await expect(this.errorState).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Click retry button (in error state)
   */
  async clickRetry(): Promise<void> {
    await expect(this.retryButton).toBeVisible();
    await this.retryButton.click();
  }

  /**
   * Scroll to load more feeds (infinite scroll)
   */
  async loadMoreFeeds(): Promise<void> {
    const _initialCount = await this.getFeedCount();
    await this.scrollToBottom();

    // Wait for new feeds to potentially load
    await this.page.waitForTimeout(1000);

    // Check if loading indicator appears/disappears
    await expect(this.loadingIndicator)
      .toBeVisible({ timeout: 2000 })
      .catch(() => {});
  }

  /**
   * Get feed card by ID
   */
  getFeedCardById(feedId: string): Locator {
    return this.page.getByTestId(`desktop-feed-card-${feedId}`);
  }
}
