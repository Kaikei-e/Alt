import { type Locator, type Page, expect } from '@playwright/test';

export class LoginPage {
  readonly page: Page;
  readonly emailInput: Locator;
  readonly passwordInput: Locator;
  readonly submitButton: Locator;
  readonly loginLink: Locator;

  constructor(page: Page) {
    this.page = page;
    this.emailInput = page.getByLabel(/email/i);
    this.passwordInput = page.getByLabel(/password/i);
    this.submitButton = page.getByRole('button', { name: /log in|login|submit/i });
    // Adjust selector if needed based on actual DOM, backup used specific href
    this.loginLink = page.locator('a[href="/auth/login"]');
  }

  async goto() {
    await this.page.goto('/public/landing');
  }

  async navigateToLogin() {
    await this.goto();
    // Wait for the link to be visible before clicking
    await this.loginLink.waitFor({ state: 'visible' });
    await this.loginLink.click();
    try {
      await this.page.waitForURL(/\/auth\/login\?flow=/, { timeout: 5000 });
    } catch (e) {
      console.log('Current URL:', this.page.url());
      throw e;
    }
  }

  async login(email: string, password: string) {
    await this.emailInput.fill(email);
    await this.passwordInput.fill(password);
    await this.submitButton.click();
  }
}
