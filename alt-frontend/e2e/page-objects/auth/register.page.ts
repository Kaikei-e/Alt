import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

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

    // Initialize locators - adjusted for OryFlowForm rendering
    this.pageHeading = page
      .getByText(/新規登録|sign up|register|create account/i)
      .first();

    // OryFlowForm uses aria-label from Kratos flow configuration
    // Typical Kratos field names: "traits.email", "password", "traits.name"
    this.nameInput = page.getByLabel(/name|氏名|full name|お名前/i);
    this.emailInput = page.getByLabel(/email|メール|e-mail/i);
    this.passwordInput = page.getByLabel(/^password$|^パスワード$/i);
    this.confirmPasswordInput = page.getByLabel(/confirm|repeat|再入力|確認/i);

    // Submit button is rendered by OryFlowForm
    this.submitButton = page.locator('button[type="submit"]');

    this.loginLink = page.getByRole("link", { name: /log in|login|ログイン/i });

    // Error messages are rendered in red bordered boxes by OryFlowForm
    this.errorMessage = page
      .locator('[role="alert"], .chakra-alert, [style*="red"]')
      .first();
    this.successMessage = page.getByRole("status");
    this.termsCheckbox = page.getByLabel(/terms|agree|利用規約|同意/i);
  }

  /**
   * Navigate to register page
   */
  async goto(): Promise<void> {
    // Navigate and wait for Kratos flow initialization
    await this.page.goto("/auth/register", { waitUntil: "networkidle" });

    // Wait for redirect and flow initialization (may redirect to Kratos then back)
    await this.page
      .waitForURL("**/auth/register?flow=**", { timeout: 15000 })
      .catch(() => {
        // If no redirect with flow, page might already have flow initialized
      });

    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    // Wait for the form container to be visible
    await this.page
      .waitForSelector('form, [role="form"]', { timeout: 15000 })
      .catch(() => {
        // Fallback: wait for specific elements
      });

    // Wait for critical elements - use more flexible timeout
    await expect(this.emailInput).toBeVisible({ timeout: 10000 });
    await expect(this.submitButton).toBeVisible({ timeout: 10000 });
  }

  /**
   * Register with full credentials
   */
  async register(
    email: string,
    password: string,
    name?: string,
    confirmPassword?: string,
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
        (url) => !url.pathname.includes("/auth/register"),
        {
          timeout: 10000,
        },
      );
    } catch {
      // If no redirect, check for success message
      await expect(this.successMessage).toBeVisible({ timeout: 5000 });
    }
  }
}
