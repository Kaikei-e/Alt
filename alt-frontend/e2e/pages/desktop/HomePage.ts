import { type Locator, type Page, expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Desktop Home Page Object
 * Uses data-testid selectors for stability
 */
export class HomePage extends BasePage {
  readonly homeContainer: Locator;
  readonly dashboardHeader: Locator;
  readonly feedCards: Locator;
  readonly statsGrid: Locator;
  readonly activityFeed: Locator;
  readonly quickActions: Locator;
  readonly ctaContainer: Locator;

  constructor(page: Page) {
    super(page);
    // Use data-testid selectors
    this.homeContainer = page.getByTestId("desktop-home-container");
    this.dashboardHeader = page.getByText("Dashboard Overview");
    this.feedCards = page.locator('[data-testid^="desktop-feed-card-"]');
    this.statsGrid = page.getByTestId("stats-grid");
    this.activityFeed = page.getByTestId("activity-feed");
    this.quickActions = page.getByTestId("quick-actions-panel");
    this.ctaContainer = page.getByTestId("cta-container");
  }

  /**
   * Navigate to desktop home page
   */
  async goto(): Promise<void> {
    await this.navigateTo("/desktop/home");
  }

  /**
   * Wait for home page to load
   */
  async waitForReady(timeout = 10000): Promise<void> {
    await expect(this.homeContainer.or(this.dashboardHeader)).toBeVisible({
      timeout,
    });
  }

  /**
   * Wait for feeds to load (legacy method)
   */
  async waitForFeeds(timeout = 10000): Promise<void> {
    await expect(this.dashboardHeader.or(this.feedCards.first())).toBeVisible({
      timeout,
    });
  }

  /**
   * Get number of visible feed cards
   */
  async getFeedCount(): Promise<number> {
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
   * Check if stats grid is visible
   */
  async hasStatsGrid(): Promise<boolean> {
    try {
      await expect(this.statsGrid).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if activity feed is visible
   */
  async hasActivityFeed(): Promise<boolean> {
    try {
      await expect(this.activityFeed).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
