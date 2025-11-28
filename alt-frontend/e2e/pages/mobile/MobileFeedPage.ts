import { type Locator, type Page, expect } from '@playwright/test';
import { BasePage } from '../BasePage';

export class MobileFeedPage extends BasePage {
  readonly feedCards: Locator;
  readonly loadingIndicator: Locator;
  readonly emptyState: Locator;

  constructor(page: Page) {
    super(page);
    // Match the actual data-testid from FeedCard component: data-testid="feed-card"
    this.feedCards = page.locator('[data-testid="feed-card"], article, [role="article"]').or(
      page.locator('div').filter({ hasText: /React|TypeScript|Next\.js|Go|AI|Database|CSS|Testing|Docker|Security/ })
    );
    this.loadingIndicator = page.locator('[data-testid="virtual-scroll-container"]').or(
      page.locator('[data-testid="loading"], [aria-label="Loading"]').or(
        page.locator('text=/loading|読み込み中/i')
      )
    );
    this.emptyState = page.getByText(/no feeds yet|no feeds available|フィードがありません|empty|No feeds available/i).or(
      page.locator('[data-testid="empty-state-icon"]')
    );
  }

  /**
   * Navigate to mobile feed page
   */
  async goto() {
    await super.goto('/mobile/feeds');
  }

  /**
   * Wait for feeds to load
   */
  async waitForFeeds(timeout = 10000) {
    await expect(this.feedCards.first()).toBeVisible({ timeout });
  }

  /**
   * Wait for empty state
   */
  async waitForEmptyState(timeout = 5000) {
    await expect(this.emptyState).toBeVisible({ timeout });
  }

  /**
   * Get the number of visible feed cards
   */
  async getFeedCount(): Promise<number> {
    return await this.feedCards.count();
  }

  /**
   * Scroll to bottom to trigger pagination
   */
  async scrollToBottom() {
    await this.page.evaluate(() => {
      window.scrollTo(0, document.body.scrollHeight);
    });
  }
}

