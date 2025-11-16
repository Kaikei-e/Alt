import { expect, type Locator, type Page } from "@playwright/test";
import { checkA11y, injectAxe } from "axe-playwright";

/**
 * Base Page Object class that all page objects should extend.
 * Provides common functionality for navigation, waiting, and accessibility checks.
 */
export abstract class BasePage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /**
   * Navigate to the page
   * Must be implemented by subclasses
   */
  abstract goto(): Promise<void>;

  /**
   * Wait for page to be fully loaded
   * Must be implemented by subclasses with specific checks
   */
  abstract waitForLoad(): Promise<void>;

  /**
   * Check if page is displayed correctly
   * Can be overridden by subclasses for specific checks
   */
  async isDisplayed(): Promise<boolean> {
    // Default implementation - checks if page is visible
    const body = this.page.locator("body");
    return await body.isVisible();
  }

  /**
   * Take screenshot with custom name
   */
  async screenshot(name: string): Promise<void> {
    await this.page.screenshot({
      path: `screenshots/${name}.png`,
      fullPage: true,
    });
  }

  /**
   * Check accessibility using axe-core
   * @param options - Optional axe configuration
   */
  async checkA11y(options?: {
    detailedReport?: boolean;
    detailedReportOptions?: { html?: boolean };
    rules?: Record<string, { enabled: boolean }>;
  }): Promise<void> {
    await injectAxe(this.page);
    await checkA11y(this.page, undefined, {
      detailedReport: options?.detailedReport ?? false,
      detailedReportOptions: options?.detailedReportOptions,
      axeOptions: {
        rules: options?.rules || {},
      },
    });
  }

  /**
   * Wait for network to be idle
   */
  async waitForNetworkIdle(): Promise<void> {
    await this.page.waitForLoadState("networkidle");
  }

  /**
   * Wait for a specific element to be visible
   */
  async waitForElement(locator: Locator, timeout = 10000): Promise<void> {
    await expect(locator).toBeVisible({ timeout });
  }

  /**
   * Get current URL
   */
  getCurrentUrl(): string {
    return this.page.url();
  }

  /**
   * Get page title
   */
  async getTitle(): Promise<string> {
    return await this.page.title();
  }

  /**
   * Reload the page
   */
  async reload(): Promise<void> {
    await this.page.reload();
    await this.waitForLoad();
  }

  /**
   * Go back in browser history
   */
  async goBack(): Promise<void> {
    await this.page.goBack();
    await this.waitForLoad();
  }

  /**
   * Go forward in browser history
   */
  async goForward(): Promise<void> {
    await this.page.goForward();
    await this.waitForLoad();
  }

  /**
   * Wait for a specific URL pattern
   */
  async waitForUrl(
    urlPattern: string | RegExp,
    timeout = 10000,
  ): Promise<void> {
    await this.page.waitForURL(urlPattern, { timeout });
  }

  /**
   * Check if an element exists (doesn't need to be visible)
   */
  async elementExists(locator: Locator): Promise<boolean> {
    try {
      await locator.waitFor({ state: "attached", timeout: 1000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Scroll to element
   */
  async scrollToElement(locator: Locator): Promise<void> {
    await locator.scrollIntoViewIfNeeded();
  }

  /**
   * Scroll to top of page
   */
  async scrollToTop(): Promise<void> {
    await this.page.evaluate(() => window.scrollTo(0, 0));
  }

  /**
   * Scroll to bottom of page
   */
  async scrollToBottom(): Promise<void> {
    await this.page.evaluate(() =>
      window.scrollTo(0, document.body.scrollHeight),
    );
  }

  /**
   * Get viewport size
   */
  getViewportSize(): { width: number; height: number } | null {
    return this.page.viewportSize();
  }

  /**
   * Set viewport size
   */
  async setViewportSize(width: number, height: number): Promise<void> {
    await this.page.setViewportSize({ width, height });
  }

  /**
   * Wait for a specific amount of time (use sparingly)
   */
  async wait(milliseconds: number): Promise<void> {
    await this.page.waitForTimeout(milliseconds);
  }

  /**
   * Fill input field by label
   */
  async fillByLabel(label: string, value: string): Promise<void> {
    await this.page.getByLabel(label).fill(value);
  }

  /**
   * Click button by role
   */
  async clickButton(name: string | RegExp): Promise<void> {
    await this.page.getByRole("button", { name }).click();
  }

  /**
   * Check if text is visible on page
   */
  async isTextVisible(text: string | RegExp): Promise<boolean> {
    try {
      await expect(this.page.getByText(text)).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get error message from page (if any)
   */
  async getErrorMessage(): Promise<string | null> {
    const errorLocators = [
      this.page.getByRole("alert"),
      this.page.getByText(/error/i),
      this.page.getByText(/failed/i),
    ];

    for (const locator of errorLocators) {
      try {
        const text = await locator.first().textContent({ timeout: 1000 });
        if (text) return text.trim();
      } catch {
        // Continue to next locator
      }
    }

    return null;
  }

  /**
   * Check if loading indicator is visible
   */
  async isLoading(): Promise<boolean> {
    const loadingLocators = [
      this.page.getByRole("status"),
      this.page.getByText(/loading/i),
      this.page.locator('[aria-busy="true"]'),
    ];

    for (const locator of loadingLocators) {
      try {
        await expect(locator).toBeVisible({ timeout: 1000 });
        return true;
      } catch {
        // Continue to next locator
      }
    }

    return false;
  }

  /**
   * Wait for loading to complete
   */
  async waitForLoadingToComplete(timeout = 30000): Promise<void> {
    const startTime = Date.now();

    while (Date.now() - startTime < timeout) {
      if (!(await this.isLoading())) {
        return;
      }
      await this.wait(100);
    }

    throw new Error("Loading did not complete within timeout");
  }
}
