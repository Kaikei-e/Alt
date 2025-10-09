import { Page, expect } from "@playwright/test";
import { BasePage } from "./BasePage";

/**
 * Desktop page object model for navigation and common desktop elements
 */
export class DesktopPage extends BasePage {
  constructor(page: Page) {
    super(page);
  }

  /**
   * Navigate to desktop home page
   */
  async navigateToHome() {
    await this.goto("/desktop/home");
  }

  /**
   * Navigate to feeds page
   */
  async navigateToFeeds() {
    await this.goto("/desktop/feeds");
  }

  /**
   * Navigate to articles page
   */
  async navigateToArticles() {
    await this.goto("/desktop/articles");
  }

  /**
   * Navigate to settings page
   */
  async navigateToSettings() {
    await this.goto("/desktop/settings");
  }

  /**
   * Navigate to feeds register page
   */
  async navigateToFeedsRegister() {
    await this.goto("/desktop/feeds/register");
  }

  /**
   * Navigate to articles search page
   */
  async navigateToArticlesSearch() {
    await this.goto("/desktop/articles/search");
  }

  /**
   * Click navigation link by name
   */
  async clickNavLink(linkName: "home" | "feeds" | "articles" | "settings") {
    const testId = `desktop-nav-link-${linkName}`;
    const link = this.page.getByTestId(testId);

    await expect(link).toBeVisible({ timeout: 5000 });
    await link.click();
    await this.page.waitForLoadState("domcontentloaded");
  }

  /**
   * Verify we are on the correct desktop page
   */
  async verifyOnDesktopPage(pageName: string) {
    const pattern =
      pageName === "home"
        ? /\/desktop\/(home|dashboard)/
        : new RegExp(`/desktop/${pageName}`);

    await expect(this.page).toHaveURL(pattern);

    const container =
      pageName === "home"
        ? this.page.getByTestId("desktop-home-container")
        : this.page.locator(`[data-testid="desktop-${pageName}"]`).first();

    await expect(container).toBeVisible({ timeout: 7000 });
  }

  /**
   * Verify navigation menu is visible
   */
  async verifyNavigationVisible() {
    const navigation = this.page.locator(
      '[data-testid="desktop-navigation"], nav[aria-label="Main navigation"], [aria-label="Main navigation"]',
    );
    await expect(navigation.first()).toBeVisible({ timeout: 7000 });
  }

  /**
   * Wait for page to be authenticated (not redirected to landing/login)
   */
  async waitForAuthenticated() {
    await expect(this.page).not.toHaveURL(/\/public\/landing/);
    await expect(this.page).not.toHaveURL(/\/auth\/login/);
    await expect(this.page.getByTestId("desktop-shell")).toBeVisible({
      timeout: 7000,
    });
  }

  /**
   * Check if user is logged in by looking for user menu or logout button
   */
  async isLoggedIn(): Promise<boolean> {
    try {
      if (await this.elementExists('[data-testid="user-menu"]')) {
        await expect(
          this.page.locator('[data-testid="user-menu"]'),
        ).toBeVisible({ timeout: 2000 });
        return true;
      }
      const logoutButton = this.page.getByRole("button", {
        name: /logout|ログアウト|sign out/i,
      });
      if ((await logoutButton.count()) > 0) {
        await expect(logoutButton).toBeVisible({ timeout: 2000 });
        return true;
      }
      return false;
    } catch {
      return false;
    }
  }

  /**
   * Perform logout if logout button exists
   */
  async logout() {
    const logoutButton = this.page.getByRole("button", {
      name: /logout|ログアウト|sign out/i,
    });

    if ((await logoutButton.count()) > 0) {
      await logoutButton.click();
      await this.waitForUrl(/\/public\/landing/, { timeout: 10000 });
    }
  }

  /**
   * Verify protected route redirects to landing when not authenticated
   */
  async verifyProtectedRouteRedirect(route: string) {
    await this.goto(route);
    await this.waitForUrl(/\/public\/landing/, { timeout: 10000 });
  }

  /**
   * Navigate between pages and verify session is maintained
   */
  async testNavigation() {
    const routes = [
      "/desktop/home",
      "/desktop/feeds",
      "/desktop/articles",
      "/desktop/settings",
    ];

    for (const route of routes) {
      await this.goto(route);
      await this.verifyOnDesktopPage(route.split("/")[2]);
      await this.waitForAuthenticated();
    }
  }
}
