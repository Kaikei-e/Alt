import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../BasePage";

export class DesktopHomePage extends BasePage {
  readonly dashboardTitle: Locator;
  readonly sidebar: Locator;
  readonly navItems: Locator;
  readonly statsCards: Locator;
  readonly quickActions: Locator;
  readonly themeToggle: Locator;

  constructor(page: Page) {
    super(page);
    this.dashboardTitle = page.getByText("Alt Dashboard");
    this.sidebar = page.locator('[class*="glass"]').first();
    this.navItems = page.getByRole("link");
    this.statsCards = page.locator('[class*="stats"]');
    this.quickActions = page.getByText(/Add Feed|Search|Browse|Bookmarks/);
    this.themeToggle = page.locator(
      '[aria-label*="theme"], [data-testid*="theme"]',
    );
  }

  async goto(): Promise<void> {
    await this.navigateTo("/desktop/home");
  }

  async waitForReady(): Promise<void> {
    await expect(this.dashboardTitle).toBeVisible({ timeout: 15000 });
  }

  async hasSidebar(): Promise<boolean> {
    try {
      await expect(this.sidebar).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async navigateToFeeds(): Promise<void> {
    await this.page.getByRole("link", { name: /Feeds/i }).first().click();
  }

  async navigateToSettings(): Promise<void> {
    await this.page
      .getByRole("link", { name: /Settings/i })
      .first()
      .click();
  }

  async navigateToSearch(): Promise<void> {
    await this.page
      .getByRole("link", { name: /Search/i })
      .first()
      .click();
  }
}
