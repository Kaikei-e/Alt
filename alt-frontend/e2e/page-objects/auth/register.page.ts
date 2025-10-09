import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from '../base.page';

/**
 * Register Page Object
 * Represents the /auth/register page
 */
export class RegisterPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly nameInput: Locator;
  readonly emailInput: Locator;
  readonly passwordInput: Locator;
  readonly confirmPasswordInput: Locator;
  readonly submitButton: Locator;
  readonly loginLink: Locator;
  readonly errorMessage: Locator;
  readonly successMessage: Locator;
  readonly termsCheckbox: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole('heading', {
      name: /sign up|register|create account/i,
    });
    this.nameInput = page.getByLabel(/name|full name/i);
    this.emailInput = page.getByLabel(/email/i);
    this.passwordInput = page.getByLabel(/^password$/i);
    this.confirmPasswordInput = page.getByLabel(/confirm password|repeat password/i);
    this.submitButton = page.getByRole('button', {
      name: /sign up|register|create account/i,
    });
    this.loginLink = page.getByRole('link', { name: /log in|login/i });
    this.errorMessage = page.getByRole('alert');
    this.successMessage = page.getByRole('status');
    this.termsCheckbox = page.getByLabel(/terms|agree/i);
  }

  /**
   * Navigate to register page
   */
  async goto(): Promise<void> {
    await this.page.goto('/auth/register');
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await expect(this.emailInput).toBeVisible();
    await expect(this.submitButton).toBeVisible();
  }

  /**
   * Register with full credentials
   */
  async register(
    email: string,
    password: string,
    name?: string,
    confirmPassword?: string
  ): Promise<void> {
    if (name && (await this.nameInput.count()) > 0) {
      await this.nameInput.fill(name);
    }

    await this.emailInput.fill(email);
    await this.passwordInput.fill(password);

    if ((await this.confirmPasswordInput.count()) > 0) {
      await this.confirmPasswordInput.fill(confirmPassword || password);
    }

    // Accept terms if checkbox exists
    if ((await this.termsCheckbox.count()) > 0) {
      await this.termsCheckbox.check();
    }

    await this.submitButton.click();
  }

  /**
   * Fill name field
   */
  async fillName(name: string): Promise<void> {
    if ((await this.nameInput.count()) > 0) {
      await this.nameInput.fill(name);
    }
  }

  /**
   * Fill email field
   */
  async fillEmail(email: string): Promise<void> {
    await this.emailInput.fill(email);
  }

  /**
   * Fill password field
   */
  async fillPassword(password: string): Promise<void> {
    await this.passwordInput.fill(password);
  }

  /**
   * Fill confirm password field
   */
  async fillConfirmPassword(password: string): Promise<void> {
    if ((await this.confirmPasswordInput.count()) > 0) {
      await this.confirmPasswordInput.fill(password);
    }
  }

  /**
   * Accept terms and conditions
   */
  async acceptTerms(): Promise<void> {
    if ((await this.termsCheckbox.count()) > 0) {
      await this.termsCheckbox.check();
    }
  }

  /**
   * Click submit button
   */
  async clickSubmit(): Promise<void> {
    await this.submitButton.click();
  }

  /**
   * Click login link
   */
  async clickLogin(): Promise<void> {
    await this.loginLink.click();
    await this.page.waitForURL(/\/auth\/login/);
  }

  /**
   * Get error message text
   */
  async getError(): Promise<string | null> {
    try {
      await expect(this.errorMessage).toBeVisible({ timeout: 5000 });
      return await this.errorMessage.textContent();
    } catch {
      return null;
    }
  }

  /**
   * Check if error is displayed
   */
  async hasError(): Promise<boolean> {
    try {
      await expect(this.errorMessage).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get success message text
   */
  async getSuccessMessage(): Promise<string | null> {
    try {
      await expect(this.successMessage).toBeVisible({ timeout: 5000 });
      return await this.successMessage.textContent();
    } catch {
      return null;
    }
  }

  /**
   * Check if submit button is disabled
   */
  async isSubmitDisabled(): Promise<boolean> {
    return await this.submitButton.isDisabled();
  }

  /**
   * Check if submit button is enabled
   */
  async isSubmitEnabled(): Promise<boolean> {
    return await this.submitButton.isEnabled();
  }

  /**
   * Clear form fields
   */
  async clearForm(): Promise<void> {
    if ((await this.nameInput.count()) > 0) {
      await this.nameInput.clear();
    }
    await this.emailInput.clear();
    await this.passwordInput.clear();
    if ((await this.confirmPasswordInput.count()) > 0) {
      await this.confirmPasswordInput.clear();
    }
  }

  /**
   * Wait for successful registration
   */
  async waitForRegistrationSuccess(): Promise<void> {
    // Wait for redirect or success message
    try {
      await this.page.waitForURL(
        url => !url.pathname.includes('/auth/register'),
        { timeout: 10000 }
      );
    } catch {
      // If no redirect, check for success message
      await expect(this.successMessage).toBeVisible({ timeout: 5000 });
    }
  }
}
