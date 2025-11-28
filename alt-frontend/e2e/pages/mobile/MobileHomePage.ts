import { type Locator, type Page, expect } from '@playwright/test';
import { BasePage } from '../BasePage';

export class MobileHomePage extends BasePage {
  readonly feedCards: Locator;
  readonly firstFeedCard: Locator;
  readonly heroSection: Locator;

  constructor(page: Page) {
    super(page);
    this.heroSection = page.locator('[data-testid="hero"], header, [role="banner"]');
    // Match the actual data-testid from FeedCard component: data-testid="feed-card"
    this.feedCards = page.locator('[data-testid="feed-card"], article, [role="article"]').or(
      page.locator('div').filter({ hasText: /React|TypeScript|Next\.js|Go|AI|Database|CSS|Testing|Docker|Security/ })
    );
    this.firstFeedCard = this.feedCards.first();
  }

  /**
   * Navigate to mobile home page
   */
  async goto() {
    await super.goto('/mobile/feeds');
  }

  /**
   * Wait for feeds to be visible
   */
  async waitForFeeds(timeout = 10000) {
    // Wait for hero section first (it's always present)
    await expect(this.heroSection.first()).toBeVisible({ timeout: 2000 });
    // Then wait for at least one feed card
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
   * Scroll to load more feeds (mobile infinite scroll)
   */
  async scrollToLoadMore() {
    await this.page.evaluate(() => {
      window.scrollTo(0, document.body.scrollHeight);
    });
    // Wait a bit for the scroll to trigger loading
    await this.page.waitForTimeout(500);
  }
}

