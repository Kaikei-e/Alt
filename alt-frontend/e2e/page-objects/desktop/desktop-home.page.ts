import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

/**
 * Desktop Home Page Object
 * Represents the /desktop/home page
 */
export class DesktopHomePage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly sidebar: Locator;
  readonly mainContent: Locator;
  readonly rightPanel: Locator;
  readonly feedsLink: Locator;
  readonly articlesLink: Locator;
  readonly analyticsPanel: Locator;
  readonly trendingTopics: Locator;
  readonly readingStats: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole("heading", { level: 1 }).first();
    this.sidebar = page.getByRole("navigation", {
      name: /sidebar|main navigation/i,
    });
    this.mainContent = page.getByRole("main");
    this.rightPanel = page.getByRole("complementary", {
      name: /analytics|stats/i,
    });
    this.feedsLink = page.getByRole("link", { name: /feeds/i });
    this.articlesLink = page.getByRole("link", { name: /articles/i });
    this.analyticsPanel = page
      .locator('[data-testid="analytics-panel"]')
      .or(page.getByRole("region", { name: /analytics/i }));
    this.trendingTopics = page.locator('[data-testid="trending-topics"]');
    this.readingStats = page.locator('[data-testid="reading-stats"]');
  }

  /**
   * Navigate to desktop home page
   */
  async goto(): Promise<void> {
    await this.page.goto("/desktop/home");
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await expect(this.mainContent).toBeVisible();
    await this.waitForNetworkIdle();
  }

  /**
   * Navigate to feeds
   */
  async navigateToFeeds(): Promise<void> {
    await this.feedsLink.click();
    await this.page.waitForURL(/\/desktop\/feeds/);
  }

  /**
   * Navigate to articles
   */
  async navigateToArticles(): Promise<void> {
    await this.articlesLink.click();
    await this.page.waitForURL(/\/desktop\/articles/);
  }

  /**
   * Check if sidebar is visible
   */
  async isSidebarVisible(): Promise<boolean> {
    try {
      await expect(this.sidebar).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if right panel is visible
   */
  async isRightPanelVisible(): Promise<boolean> {
    try {
      await expect(this.rightPanel).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if analytics panel is visible
   */
  async hasAnalyticsPanel(): Promise<boolean> {
    return (await this.analyticsPanel.count()) > 0;
  }

  /**
   * Check if trending topics are visible
   */
  async hasTrendingTopics(): Promise<boolean> {
    return (await this.trendingTopics.count()) > 0;
  }

  /**
   * Check if reading stats are visible
   */
  async hasReadingStats(): Promise<boolean> {
    return (await this.readingStats.count()) > 0;
  }

  /**
   * Get reading stats data
   */
  async getReadingStats(): Promise<string | null> {
    if (await this.hasReadingStats()) {
      return await this.readingStats.textContent();
    }
    return null;
  }

  /**
   * Get trending topics
   */
  async getTrendingTopics(): Promise<string[]> {
    if (await this.hasTrendingTopics()) {
      return await this.trendingTopics
        .locator('li, [role="listitem"]')
        .allTextContents();
    }
    return [];
  }
}
