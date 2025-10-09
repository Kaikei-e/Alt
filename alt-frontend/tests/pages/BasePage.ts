import { Page, Locator, expect } from "@playwright/test";

/**
 * Base page class with common functionality
 */
export class BasePage {
  constructor(protected page: Page) {}

  /**
   * Navigate to a specific URL and wait for load
   */
  async goto(url: string, options?: { timeout?: number }) {
    await this.page.goto(url, options);
    await this.page.waitForLoadState("domcontentloaded");
  }

  /**
   * Wait for element to be visible
   */
  async waitForElement(
    selector: string | Locator,
    options?: { timeout?: number },
  ): Promise<Locator> {
    const element =
      typeof selector === "string" ? this.page.locator(selector) : selector;
    await expect(element).toBeVisible(options);
    return element;
  }

  /**
   * Get page title
   */
  async getTitle(): Promise<string> {
    return await this.page.title();
  }

  /**
   * Wait for URL to match pattern
   */
  async waitForUrl(pattern: string | RegExp, options?: { timeout?: number }) {
    await this.page.waitForURL(pattern, options);
  }

  /**
   * Get current URL
   */
  getCurrentUrl(): string {
    return this.page.url();
  }

  /**
   * Take screenshot
   */
  async screenshot(name: string) {
    await this.page.screenshot({ path: `test-results/${name}.png` });
  }

  /**
   * Wait for network to be idle
   */
  async waitForNetwork() {
    await this.page.waitForLoadState("domcontentloaded");
  }

  /**
   * Check if element exists
   */
  async elementExists(selector: string): Promise<boolean> {
    return (await this.page.locator(selector).count()) > 0;
  }

  /**
   * Get element by test ID
   */
  getByTestId(testId: string): Locator {
    return this.page.getByTestId(testId);
  }

  /**
   * Scroll to bottom of page
   */
  async scrollToBottom() {
    await this.page.evaluate(() =>
      window.scrollTo(0, document.body.scrollHeight),
    );
  }

  /**
   * Wait for a specific amount of time (use sparingly)
   */
  async wait(milliseconds: number) {
    await this.page.waitForTimeout(milliseconds);
  }
}
