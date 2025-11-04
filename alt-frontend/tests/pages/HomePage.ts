import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "./BasePage";

/**
 * Desktop Home Page Object
 * Represents the /desktop/home page
 * Simplified implementation aligned with /tests/pages/ structure
 */
export class HomePage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly mainContent: Locator;
  readonly feedsList: Locator;
  readonly articlesTimeline: Locator;
  readonly sidebar: Locator;
  readonly rightPanel: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators using common patterns
    this.pageHeading = page.locator("h1, h2").first();
    this.mainContent = page.locator('[data-testid="main-content"]');
    this.feedsList = page.locator('[data-testid^="feed-"]');
    this.articlesTimeline = page.locator('[data-testid="articles-timeline"]');
    this.sidebar = page.locator('[data-testid="desktop-navigation"]');
    this.rightPanel = page.locator('[data-testid="right-panel"]');
  }

  /**
   * Navigate to home page
   */
  async navigateToHome() {
    await this.goto("/desktop/home");
    await this.page.waitForLoadState("domcontentloaded", { timeout: 10000 });
  }

  /**
   * Wait for page to be fully loaded
   * Simplified: Just wait for DOM and URL
   */
  async waitForLoad() {
    await this.page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await this.page.waitForURL(/\/desktop\/(home|dashboard)/, {
      timeout: 10000,
    });
    await this.page.waitForTimeout(500);
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
   * Check if main content is loaded
   */
  async isContentLoaded(): Promise<boolean> {
    try {
      await this.page.waitForLoadState("domcontentloaded", {
        timeout: 10000,
      });
      return true;
    } catch {
      return false;
    }
  }
}
