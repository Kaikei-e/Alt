import { type Locator, type Page, expect } from '@playwright/test';
import { BasePage } from '../BasePage';

export class FeedPage extends BasePage {
  readonly feedCards: Locator;
  readonly loadMoreButton: Locator;
  readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    // Match the actual data-testid pattern from DesktopTimeline: desktop-feed-card-${feed.id}
    this.feedCards = page.locator('[data-testid^="desktop-feed-card-"], article, [role="article"]').or(
      page.locator('div').filter({ hasText: /React|TypeScript|Next\.js|Go|AI|Database|CSS|Testing|Docker|Security/ })
    );
    this.loadMoreButton = page.getByRole('button', { name: /load more|more|続きを読む/i });
    this.loadingIndicator = page.locator('[data-testid="infinite-scroll-sentinel"]').or(
      page.locator('[data-testid="loading"], [aria-label="Loading"]').or(
        page.locator('text=/loading|読み込み中|Loading more/i')
      )
    );
  }

  /**
   * Navigate to feed page
   */
  async goto() {
    await super.goto('/desktop/feeds');
  }

  /**
   * Wait for feeds to be visible
   * Uses Playwright's automatic waiting (expect().toBeVisible() waits automatically)
   */
  async waitForFeeds() {
    await expect(this.feedCards.first()).toBeVisible();
  }

  /**
   * Scroll to the bottom of the page to trigger infinite scroll
   */
  async scrollToBottom() {
    await this.page.evaluate(() => {
      window.scrollTo(0, document.body.scrollHeight);
    });
  }

  /**
   * Wait for loading indicator to appear and disappear
   */
  async waitForLoading(timeout = 5000) {
    try {
      await expect(this.loadingIndicator).toBeVisible({ timeout: 2000 });
      await expect(this.loadingIndicator).toBeHidden({ timeout });
    } catch {
      // Loading indicator might not appear if response is very fast
    }
  }

  /**
   * Get the number of visible feed cards
   */
  async getFeedCount(): Promise<number> {
    return await this.feedCards.count();
  }
  /**
   * Get the title of the first feed
   */
  async getFirstFeedTitle(): Promise<string> {
    return await this.feedCards.first().locator('h1, h2, h3, [data-testid="feed-title"]').first().textContent() || '';
  }
}

