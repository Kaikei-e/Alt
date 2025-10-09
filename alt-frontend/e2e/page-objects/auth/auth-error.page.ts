import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from '../base.page';

/**
 * Auth Error Page Object
 * Represents the /auth/error page
 */
export class AuthErrorPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly errorMessage: Locator;
  readonly errorDetails: Locator;
  readonly retryButton: Locator;
  readonly backToLoginButton: Locator;
  readonly homeButton: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole('heading', {
      name: /error|something went wrong/i,
    });
    this.errorMessage = page.getByRole('alert').or(
      page.getByText(/error|failed|something went wrong/i).first()
    );
    this.errorDetails = page.locator('[data-testid="error-details"]');
    this.retryButton = page.getByRole('button', { name: /retry|try again/i });
    this.backToLoginButton = page.getByRole('link', {
      name: /back to login|login/i,
    });
    this.homeButton = page.getByRole('link', { name: /home|back home/i });
  }

  /**
   * Navigate to error page
   */
  async goto(): Promise<void> {
    await this.page.goto('/auth/error');
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.errorMessage).toBeVisible();
  }

  /**
   * Get error message text
   */
  async getErrorMessage(): Promise<string> {
    return (await this.errorMessage.textContent()) || '';
  }

  /**
   * Get error details
   */
  async getErrorDetails(): Promise<string | null> {
    if ((await this.errorDetails.count()) > 0) {
      return await this.errorDetails.textContent();
    }
    return null;
  }

  /**
   * Click retry button
   */
  async clickRetry(): Promise<void> {
    await this.retryButton.click();
  }

  /**
   * Click back to login button
   */
  async clickBackToLogin(): Promise<void> {
    await this.backToLoginButton.click();
    await this.page.waitForURL(/\/auth\/login/);
  }

  /**
   * Click home button
   */
  async clickHome(): Promise<void> {
    await this.homeButton.click();
    await this.page.waitForURL(/\/home|\/public\/landing/);
  }

  /**
   * Check if retry button is visible
   */
  async hasRetryButton(): Promise<boolean> {
    return (await this.retryButton.count()) > 0;
  }

  /**
   * Check if back to login button is visible
   */
  async hasBackToLoginButton(): Promise<boolean> {
    return (await this.backToLoginButton.count()) > 0;
  }
}
