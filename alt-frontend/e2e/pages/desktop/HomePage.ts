import { type Locator, type Page, expect } from '@playwright/test';
import { BasePage } from '../BasePage';

export class HomePage extends BasePage {
  readonly dashboardHeader: Locator;
  readonly feedCards: Locator;
  readonly firstFeedCard: Locator;

  constructor(page: Page) {
    super(page);
    this.dashboardHeader = page.getByText('Dashboard Overview');
    // Match the actual data-testid pattern from DesktopTimeline: desktop-feed-card-${feed.id}
    this.feedCards = page.locator('[data-testid^="desktop-feed-card-"], article, [role="article"]').or(
      page.locator('div').filter({ hasText: /React|TypeScript|Next\.js|Go|AI|Database|CSS|Testing|Docker|Security/ })
    );
    this.firstFeedCard = this.feedCards.first();
  }

  /**
   * Navigate to desktop home page
   */
  async goto() {
    await super.goto('/desktop/home');
  }

  /**
   * Wait for feed cards to be visible
   */
  async waitForFeeds(timeout = 10000) {
    await expect(this.dashboardHeader).toBeVisible({ timeout });
    // Wait for at least one feed card
    await expect(this.feedCards.first()).toBeVisible({ timeout });
  }

  /**
   * Get the number of visible feed cards
   */
  async getFeedCount(): Promise<number> {
    return await this.feedCards.count();
  }

  /**
   * Click on the first feed card
   */
  async clickFirstFeed() {
    await this.firstFeedCard.click();
  }

  /**
   * Get the title of the first feed
   */
  async getFirstFeedTitle(): Promise<string> {
    return await this.firstFeedCard.locator('h1, h2, h3, [data-testid="feed-title"]').first().textContent() || '';
  }
}

