import { Page, expect } from '@playwright/test';

/**
 * Login Page Object Model
 */
export class LoginPage {
  constructor(private page: Page) {}
  
  async goto() {
    await this.page.goto('/auth/login');
  }
  
  async login(email: string, password: string) {
    await expect(this.page.getByLabel('Email')).toBeVisible();
    await this.page.getByLabel('Email').fill(email);
    await this.page.getByLabel('Password').fill(password);
    await this.page.getByRole('button', { name: /sign in/i }).click();
  }
  
  async expectErrorMessage(message?: string) {
    if (message) {
      await expect(this.page.getByText(message)).toBeVisible();
    } else {
      await expect(this.page.getByText(/credentials are invalid/i)).toBeVisible();
    }
  }
}

/**
 * Desktop Navigation Page Object Model
 */
export class DesktopNavigationPage {
  constructor(private page: Page) {}
  
  async expectToBeOnHomePage() {
    await expect(this.page).toHaveURL('/desktop/home');
  }
  
  async expectToBeOnFeedsPage() {
    await expect(this.page).toHaveURL('/desktop/feeds');
  }
  
  async expectToBeOnSettingsPage() {
    await expect(this.page).toHaveURL('/desktop/settings');
  }
  
  async navigateToFeeds() {
    await this.page.goto('/desktop/feeds');
  }
  
  async navigateToSettings() {
    await this.page.goto('/desktop/settings');
  }
}