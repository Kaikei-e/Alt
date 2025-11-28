import { type Page, expect } from '@playwright/test';

export class BasePage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /**
   * Navigate to a specific URL
   */
  async goto(url: string) {
    await this.page.goto(url, { waitUntil: 'domcontentloaded' });
    // Don't wait for networkidle as it can timeout with mocks or long-running requests
    // Instead, wait for the page to be interactive
    await this.page.waitForLoadState('load', { timeout: 10000 }).catch(() => {
      // If load state times out, continue anyway - the page might still be functional
    });
  }

  /**
   * Wait for page to be fully loaded
   * @deprecated Use specific wait methods instead (e.g., waitForFeeds, waitForArticle)
   */
  async waitForLoad() {
    // Use a shorter timeout and fallback to domcontentloaded if networkidle fails
    try {
      await this.page.waitForLoadState('networkidle', { timeout: 5000 });
    } catch {
      // Fallback to load state if networkidle times out
      await this.page.waitForLoadState('load', { timeout: 5000 });
    }
  }

  /**
   * Get page title
   */
  async getTitle(): Promise<string> {
    return await this.page.title();
  }

  /**
   * Get current URL
   */
  getUrl(): string {
    return this.page.url();
  }

  /**
   * Wait for a specific text to be visible
   */
  async waitForText(text: string, timeout = 5000) {
    await expect(this.page.getByText(text)).toBeVisible({ timeout });
  }

  /**
   * Take a screenshot (useful for debugging)
   */
  async screenshot(path: string) {
    await this.page.screenshot({ path, fullPage: true });
  }
}

