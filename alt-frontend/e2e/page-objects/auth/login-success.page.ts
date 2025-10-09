import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from '../base.page';

/**
 * Login Success Page Object
 * Represents the /auth/login/success page
 */
export class LoginSuccessPage extends BasePage {
  // Locators
  readonly successMessage: Locator;
  readonly continueButton: Locator;
  readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.successMessage = page.getByRole('status').or(
      page.getByText(/success|logged in|welcome/i).first()
    );
    this.continueButton = page.getByRole('button', {
      name: /continue|proceed|go to dashboard/i,
    });
    this.loadingIndicator = page.getByRole('status', { name: /loading/i }).or(
      page.locator('[aria-busy="true"]')
    );
  }

  /**
   * Navigate to success page
   */
  async goto(): Promise<void> {
    await this.page.goto('/auth/login/success');
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    // Wait for either success message or automatic redirect
    try {
      await expect(this.successMessage).toBeVisible({ timeout: 5000 });
    } catch {
      // Page might auto-redirect, which is also valid
    }
  }

  /**
   * Get success message text
   */
  async getSuccessMessage(): Promise<string | null> {
    try {
      return await this.successMessage.textContent();
    } catch {
      return null;
    }
  }

  /**
   * Click continue button
   */
  async clickContinue(): Promise<void> {
    if ((await this.continueButton.count()) > 0) {
      await this.continueButton.click();
    }
  }

  /**
   * Wait for automatic redirect
   */
  async waitForRedirect(timeout = 10000): Promise<void> {
    await this.page.waitForURL(
      url => !url.pathname.includes('/auth/login/success'),
      { timeout }
    );
  }

  /**
   * Check if loading indicator is visible
   */
  async isLoading(): Promise<boolean> {
    try {
      await expect(this.loadingIndicator).toBeVisible({ timeout: 1000 });
      return true;
    } catch {
      return false;
    }
  }
}
