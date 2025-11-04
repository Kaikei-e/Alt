import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

/**
 * Home Page Object
 * Represents the /home page (root home)
 */
export class HomePage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly welcomeMessage: Locator;
  readonly desktopCard: Locator;
  readonly mobileCard: Locator;
  readonly logoutButton: Locator;
  readonly settingsLink: Locator;
  readonly themeToggle: Locator;
  readonly userMenu: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole("heading", { level: 1 }).first();
    this.welcomeMessage = page.getByText(/welcome/i);
    this.desktopCard = page.getByRole("link", { name: /desktop/i });
    this.mobileCard = page.getByRole("link", { name: /mobile/i });
    this.logoutButton = page.getByRole("button", { name: /log out|logout/i });
    this.settingsLink = page.getByRole("link", { name: /settings/i });
    this.themeToggle = page.getByRole("button", { name: /theme|dark mode|light mode/i });
    this.userMenu = page.getByRole("button", { name: /user menu|account/i });
  }

  /**
   * Navigate to home page
   */
  async goto(): Promise<void> {
    await this.page.goto("/home");
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await this.waitForNetworkIdle();
  }

  /**
   * Navigate to desktop view
   */
  async goToDesktop(): Promise<void> {
    await this.desktopCard.click();
    await this.page.waitForURL(/\/desktop/);
  }

  /**
   * Navigate to mobile view
   */
  async goToMobile(): Promise<void> {
    await this.mobileCard.click();
    await this.page.waitForURL(/\/mobile/);
  }

  /**
   * Logout
   */
  async logout(): Promise<void> {
    await this.logoutButton.click();
    await this.page.waitForURL(/\/public\/landing|\/auth\/login/);
  }

  /**
   * Navigate to settings
   */
  async goToSettings(): Promise<void> {
    await this.settingsLink.click();
    await this.page.waitForURL(/\/settings/);
  }

  /**
   * Toggle theme
   */
  async toggleTheme(): Promise<void> {
    if ((await this.themeToggle.count()) > 0) {
      await this.themeToggle.click();
    }
  }

  /**
   * Open user menu
   */
  async openUserMenu(): Promise<void> {
    if ((await this.userMenu.count()) > 0) {
      await this.userMenu.click();
    }
  }

  /**
   * Check if welcome message is visible
   */
  async hasWelcomeMessage(): Promise<boolean> {
    try {
      await expect(this.welcomeMessage).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get welcome message text
   */
  async getWelcomeMessage(): Promise<string | null> {
    if (await this.hasWelcomeMessage()) {
      return await this.welcomeMessage.textContent();
    }
    return null;
  }
}
