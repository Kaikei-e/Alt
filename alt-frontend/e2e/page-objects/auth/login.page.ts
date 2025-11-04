import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

/**
 * Login Page Object
 * Represents the /auth/login page
 */
export class LoginPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly emailInput: Locator;
  readonly passwordInput: Locator;
  readonly submitButton: Locator;
  readonly registerLink: Locator;
  readonly errorMessage: Locator;
  readonly forgotPasswordLink: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators using semantic roles
    this.pageHeading = page.getByRole("heading", { name: /log in|login/i });
    this.emailInput = page.getByLabel(/email/i);
    this.passwordInput = page.getByLabel(/password/i);
    this.submitButton = page.getByRole("button", { name: /log in|login|submit/i });
    this.registerLink = page.getByRole("link", { name: /sign up|register/i });
    this.errorMessage = page.getByRole("alert");
    this.forgotPasswordLink = page.getByRole("link", {
      name: /forgot password/i,
    });
  }

  /**
   * Navigate to login page
   */
  async goto(): Promise<void> {
    await this.page.goto("/auth/login");
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await expect(this.emailInput).toBeVisible();
    await expect(this.passwordInput).toBeVisible();
    await expect(this.submitButton).toBeVisible();
  }

  /**
   * Login with credentials
   */
  async login(email: string, password: string): Promise<void> {
    await this.emailInput.fill(email);
    await this.passwordInput.fill(password);
    await this.submitButton.click();
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
   * Click submit button
   */
  async clickSubmit(): Promise<void> {
    await this.submitButton.click();
  }

  /**
   * Click register link
   */
  async clickRegister(): Promise<void> {
    await this.registerLink.click();
    await this.page.waitForURL(/\/auth\/register/);
  }

  /**
   * Click forgot password link
   */
  async clickForgotPassword(): Promise<void> {
    await this.forgotPasswordLink.click();
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
    await this.emailInput.clear();
    await this.passwordInput.clear();
  }

  /**
   * Check if login form is displayed
   */
  async isLoginFormDisplayed(): Promise<boolean> {
    try {
      await expect(this.emailInput).toBeVisible({ timeout: 2000 });
      await expect(this.passwordInput).toBeVisible({ timeout: 2000 });
      await expect(this.submitButton).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Wait for successful login redirect
   */
  async waitForLoginSuccess(): Promise<void> {
    // Wait for redirect away from login page
    await this.page.waitForURL((url) => !url.pathname.includes("/auth/login"), {
      timeout: 10000,
    });
  }
}
