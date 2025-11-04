import { expect, type Locator, type Page } from "@playwright/test";
import { safeClick, safeFill, waitForFormReady, waitForTextContent } from "../utils/waitConditions";
import { BasePage } from "./BasePage";

/**
 * Login page object model
 */
export class LoginPage extends BasePage {
  private readonly emailInput: Locator;
  private readonly passwordInput: Locator;
  private readonly signInButton: Locator;
  private readonly errorMessage: Locator;
  private readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    // Use actual Kratos form field names
    this.emailInput = this.page.locator('input[name="identifier"]');
    this.passwordInput = this.page.locator('input[name="password"]');
    this.signInButton = this.page.locator('button[type="submit"]');
    this.errorMessage = this.page.locator(
      '[data-testid="error-message"], .error-message, [role="alert"]'
    );
    this.loadingIndicator = this.page.getByText(/loading|準備しています/i);
  }

  /**
   * Navigate to login page
   */
  async navigateToLogin() {
    await this.goto("/auth/login");
  }

  /**
   * Wait for login form to be ready
   */
  async waitForForm(timeout = 10000) {
    await waitForFormReady(this.page, "form", timeout);
    await this.waitForElement(this.emailInput, { timeout });
    await this.waitForElement(this.passwordInput, { timeout });
    await this.waitForElement(this.signInButton, { timeout });
  }

  /**
   * Fill email field
   */
  async fillEmail(email: string) {
    await safeFill(this.emailInput, email);
  }

  /**
   * Fill password field
   */
  async fillPassword(password: string) {
    await safeFill(this.passwordInput, password);
  }

  /**
   * Click sign in button
   */
  async clickSignIn() {
    await safeClick(this.signInButton);
  }

  /**
   * Complete login flow with credentials
   */
  async login(email = "test@example.com", password = "password123") {
    await this.waitForForm();
    await this.fillEmail(email);
    await this.fillPassword(password);
    await this.clickSignIn();
  }

  /**
   * Wait for login error to appear
   */
  async waitForError(timeout = 5000) {
    await this.waitForElement(this.errorMessage, { timeout });
    return await this.errorMessage.textContent();
  }

  /**
   * Check if loading state is visible
   */
  async isLoading(): Promise<boolean> {
    try {
      await expect(this.loadingIndicator).toBeVisible({ timeout: 1000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Wait for redirect after login
   */
  async waitForLoginSuccess(expectedUrl?: string | RegExp, timeout = 30000) {
    if (expectedUrl) {
      await this.waitForUrl(expectedUrl, { timeout });
    } else {
      // Wait for redirect away from auth pages
      await this.waitForUrl(/\/(desktop\/home|home|mobile)/, { timeout });
    }
  }

  /**
   * Complete full login flow and wait for success
   */
  async performLogin(
    email = "test@example.com",
    password = "password123",
    expectedUrl?: string | RegExp
  ) {
    await this.login(email, password);
    await this.waitForLoginSuccess(expectedUrl);
  }

  /**
   * Verify login page elements are present
   */
  async verifyLoginPageElements() {
    await expect(this.page.getByRole("heading", { name: "Welcome back" })).toBeVisible();
    await expect(this.emailInput).toBeVisible();
    await expect(this.passwordInput).toBeVisible();
    await expect(this.signInButton).toBeVisible();
  }
}
