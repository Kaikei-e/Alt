import { type Page, type Locator, expect } from '@playwright/test';

/**
 * Base Page Object class with Web-first assertion patterns
 * Following Playwright best practices: https://playwright.dev/docs/best-practices
 */
export abstract class BasePage {
  constructor(protected readonly page: Page) {}

  /**
   * Navigate to page - subclasses should implement their own goto()
   */
  protected async navigateTo(path: string): Promise<void> {
    await this.page.goto(path);
    await this.waitForPageReady();
  }

  /**
   * Wait for page to be ready (DOM content loaded)
   * Note: networkidle is avoided as it's flaky
   */
  protected async waitForPageReady(): Promise<void> {
    await this.page.waitForLoadState('domcontentloaded');
    // Short timeout for networkidle - don't fail if it times out
    await this.page
      .waitForLoadState('networkidle', { timeout: 5000 })
      .catch(() => {});
  }

  /**
   * Get current URL
   */
  getUrl(): string {
    return this.page.url();
  }

  /**
   * Get page title
   */
  async getTitle(): Promise<string> {
    return this.page.title();
  }

  /**
   * Scroll to bottom of page (for infinite scroll)
   */
  async scrollToBottom(): Promise<void> {
    await this.page.evaluate(() => {
      window.scrollTo(0, document.body.scrollHeight);
    });
    // Short delay to allow content to load
    await this.page.waitForTimeout(300);
  }

  /**
   * Scroll to specific element
   */
  async scrollToElement(locator: Locator): Promise<void> {
    await locator.scrollIntoViewIfNeeded();
  }

  /**
   * Web-first assertion: wait for element to be visible
   */
  protected async assertVisible(
    locator: Locator,
    timeout = 10000,
  ): Promise<void> {
    await expect(locator).toBeVisible({ timeout });
  }

  /**
   * Web-first assertion: wait for element to be hidden
   */
  protected async assertHidden(
    locator: Locator,
    timeout = 10000,
  ): Promise<void> {
    await expect(locator).toBeHidden({ timeout });
  }

  /**
   * Web-first assertion: wait for element to contain text
   */
  protected async assertContainsText(
    locator: Locator,
    text: string | RegExp,
    timeout = 10000,
  ): Promise<void> {
    await expect(locator).toContainText(text, { timeout });
  }

  /**
   * Web-first assertion: wait for element count
   */
  protected async assertCount(
    locator: Locator,
    count: number,
    timeout = 10000,
  ): Promise<void> {
    await expect(locator).toHaveCount(count, { timeout });
  }

  /**
   * Web-first assertion: wait for element to have attribute
   */
  protected async assertAttribute(
    locator: Locator,
    name: string,
    value: string | RegExp,
    timeout = 10000,
  ): Promise<void> {
    await expect(locator).toHaveAttribute(name, value, { timeout });
  }

  /**
   * Wait for API response
   */
  async waitForApiResponse(
    urlPattern: string | RegExp,
    timeout = 10000,
  ): Promise<void> {
    await this.page.waitForResponse(
      (response) => {
        const url = response.url();
        return typeof urlPattern === 'string'
          ? url.includes(urlPattern)
          : urlPattern.test(url);
      },
      { timeout },
    );
  }

  /**
   * Wait for navigation to complete
   */
  async waitForNavigation(timeout = 10000): Promise<void> {
    await this.page.waitForLoadState('domcontentloaded', { timeout });
  }

  /**
   * Take a screenshot (for debugging)
   */
  async screenshot(name: string): Promise<void> {
    await this.page.screenshot({
      path: `screenshots/${name}.png`,
      fullPage: true,
    });
  }

  /**
   * Get text content of an element
   */
  protected async getText(locator: Locator): Promise<string> {
    await expect(locator).toBeVisible();
    return (await locator.textContent()) ?? '';
  }

  /**
   * Click element and wait for navigation
   */
  protected async clickAndNavigate(locator: Locator): Promise<void> {
    await Promise.all([
      this.page.waitForLoadState('domcontentloaded'),
      locator.click(),
    ]);
  }
}
