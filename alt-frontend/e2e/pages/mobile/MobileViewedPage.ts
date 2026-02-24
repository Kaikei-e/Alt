import { type Locator, type Page, expect } from "@playwright/test";
import { BasePage } from "../BasePage";

export class MobileViewedPage extends BasePage {
  readonly scrollContainer: Locator;
  readonly skeletonContainer: Locator;
  readonly pageTitle: Locator;
  readonly feedList: Locator;
  readonly emptyState: Locator;
  readonly loadingIndicator: Locator;
  readonly infiniteScrollSentinel: Locator;

  constructor(page: Page) {
    super(page);
    this.scrollContainer = page.getByTestId("read-feeds-scroll-container");
    this.skeletonContainer = page.getByTestId("read-feeds-skeleton-container");
    this.pageTitle = page.getByTestId("read-feeds-title");
    this.feedList = page.getByTestId("virtual-feed-list");
    this.emptyState = page.getByText("No History Yet");
    this.loadingIndicator = page.getByText("Loading more...");
    this.infiniteScrollSentinel = page.getByTestId("infinite-scroll-sentinel");
  }

  async goto(): Promise<void> {
    await this.navigateTo("/mobile/feeds/viewed");
  }

  async waitForReady(): Promise<void> {
    // Wait for skeleton to disappear
    await expect(this.skeletonContainer)
      .toBeHidden({ timeout: 15000 })
      .catch(() => {});
    // Wait for scroll container to be visible
    await expect(this.scrollContainer).toBeVisible({ timeout: 15000 });
  }

  async hasEmptyState(): Promise<boolean> {
    try {
      await expect(this.emptyState).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async hasFeedList(): Promise<boolean> {
    try {
      await expect(this.feedList).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async getTitle(): Promise<string> {
    return (await this.pageTitle.textContent()) ?? "";
  }
}
