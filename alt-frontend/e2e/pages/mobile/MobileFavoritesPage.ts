import { type Locator, type Page, expect } from '@playwright/test';
import { BasePage } from '../BasePage';

export class MobileFavoritesPage extends BasePage {
  readonly scrollContainer: Locator;
  readonly skeletonContainer: Locator;
  readonly feedCards: Locator;
  readonly emptyState: Locator;
  readonly loadingIndicator: Locator;
  readonly infiniteScrollSentinel: Locator;

  constructor(page: Page) {
    super(page);
    this.scrollContainer = page.getByTestId('favorites-scroll-container');
    this.skeletonContainer = page.getByTestId('favorites-skeleton-container');
    this.feedCards = page.getByTestId('feed-card');
    this.emptyState = page.getByText('No feeds available');
    this.loadingIndicator = page.getByText('Loading more...');
    this.infiniteScrollSentinel = page.getByTestId('infinite-scroll-sentinel');
  }

  async goto(): Promise<void> {
    await this.navigateTo('/mobile/feeds/favorites');
  }

  async waitForReady(): Promise<void> {
    // Wait for skeleton to disappear
    await expect(this.skeletonContainer).toBeHidden({ timeout: 15000 }).catch(() => {});
    // Wait for scroll container to be visible
    await expect(this.scrollContainer).toBeVisible({ timeout: 15000 });
  }

  async getFeedCount(): Promise<number> {
    await this.page.waitForTimeout(500);
    return await this.feedCards.count();
  }

  async hasEmptyState(): Promise<boolean> {
    try {
      await expect(this.emptyState).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async hasFeeds(): Promise<boolean> {
    const count = await this.getFeedCount();
    return count > 0;
  }
}
