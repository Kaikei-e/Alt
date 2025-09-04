import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './BasePage';

/**
 * Desktop page object model for navigation and common desktop elements
 */
export class DesktopPage extends BasePage {
  private readonly navigationMenu: Locator;
  private readonly homeLink: Locator;
  private readonly feedsLink: Locator;
  private readonly articlesLink: Locator;
  private readonly settingsLink: Locator;
  private readonly userMenu: Locator;
  private readonly logoutButton: Locator;

  constructor(page: Page) {
    super(page);
    // Navigation elements - adjust selectors based on actual implementation
    this.navigationMenu = this.page.locator('[data-testid="navigation-menu"], nav');
    this.homeLink = this.page.getByRole('link', { name: /home/i });
    this.feedsLink = this.page.getByRole('link', { name: /feeds/i });
    this.articlesLink = this.page.getByRole('link', { name: /articles/i });
    this.settingsLink = this.page.getByRole('link', { name: /settings/i });
    this.userMenu = this.page.locator('[data-testid="user-menu"]');
    this.logoutButton = this.page.getByRole('button', { name: /logout|sign out/i });
  }

  /**
   * Navigate to desktop home page
   */
  async navigateToHome() {
    await this.goto('/desktop/home');
  }

  /**
   * Navigate to feeds page
   */
  async navigateToFeeds() {
    await this.goto('/desktop/feeds');
  }

  /**
   * Navigate to articles page
   */
  async navigateToArticles() {
    await this.goto('/desktop/articles');
  }

  /**
   * Navigate to settings page
   */
  async navigateToSettings() {
    await this.goto('/desktop/settings');
  }

  /**
   * Navigate to feeds register page
   */
  async navigateToFeedsRegister() {
    await this.goto('/desktop/feeds/register');
  }

  /**
   * Navigate to articles search page
   */
  async navigateToArticlesSearch() {
    await this.goto('/desktop/articles/search');
  }

  /**
   * Click navigation link by name
   */
  async clickNavLink(linkName: 'home' | 'feeds' | 'articles' | 'settings') {
    switch (linkName) {
      case 'home':
        await this.homeLink.click();
        break;
      case 'feeds':
        await this.feedsLink.click();
        break;
      case 'articles':
        await this.articlesLink.click();
        break;
      case 'settings':
        await this.settingsLink.click();
        break;
    }
    await this.waitForNetwork();
  }

  /**
   * Verify we are on the correct desktop page
   */
  async verifyOnDesktopPage(pageName: string) {
    await expect(this.page).toHaveURL(`/desktop/${pageName}`);
    await expect(this.page).toHaveTitle(/Alt/);
  }

  /**
   * Verify navigation menu is visible
   */
  async verifyNavigationVisible() {
    if (await this.elementExists('[data-testid="navigation-menu"]')) {
      await expect(this.navigationMenu).toBeVisible();
    }
  }

  /**
   * Wait for page to be authenticated (not redirected to login)
   */
  async waitForAuthenticated() {
    await expect(this.page).not.toHaveURL(/\/auth\/login/);
  }

  /**
   * Check if user is logged in by looking for user menu or logout button
   */
  async isLoggedIn(): Promise<boolean> {
    try {
      if (await this.elementExists('[data-testid="user-menu"]')) {
        await expect(this.userMenu).toBeVisible({ timeout: 2000 });
        return true;
      }
      if (await this.elementExists('[role="button"][name*="logout"], [role="button"][name*="sign out"]')) {
        await expect(this.logoutButton).toBeVisible({ timeout: 2000 });
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
    if (await this.elementExists('[role="button"][name*="logout"], [role="button"][name*="sign out"]')) {
      await this.logoutButton.click();
      await this.waitForUrl(/\/auth\/login/, { timeout: 10000 });
    }
  }

  /**
   * Verify protected route redirects to login when not authenticated
   */
  async verifyProtectedRouteRedirect(route: string) {
    await this.goto(route);
    await this.waitForUrl(/\/auth\/login\?flow=/, { timeout: 10000 });
  }

  /**
   * Navigate between pages and verify session is maintained
   */
  async testNavigation() {
    const routes = ['/desktop/home', '/desktop/feeds', '/desktop/articles', '/desktop/settings'];
    
    for (const route of routes) {
      await this.goto(route);
      await this.verifyOnDesktopPage(route.split('/')[2]);
      await this.waitForAuthenticated();
    }
  }
}