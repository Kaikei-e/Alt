import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from '../base.page';

/**
 * Landing Page Object
 * Represents the /public/landing page
 */
export class LandingPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly loginButton: Locator;
  readonly registerButton: Locator;
  readonly heroSection: Locator;
  readonly featuresSection: Locator;
  readonly ctaSection: Locator;
  readonly logo: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators based on actual page structure
    this.pageHeading = page.getByRole('heading', { name: /^Alt$/i });
    this.loginButton = page.getByRole('link', { name: /ログイン/i });
    this.registerButton = page.getByRole('link', { name: /新規登録/i });
    this.heroSection = page.locator('section').first(); // First section is hero
    this.featuresSection = page.locator('section').nth(1); // Platform stats section
    this.ctaSection = page.locator('footer'); // Footer section
    this.logo = page.getByRole('heading', { name: /^Alt$/i }); // Logo is the h1
  }

  /**
   * Navigate to landing page
   */
  async goto(): Promise<void> {
    await this.page.goto('/public/landing');
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await this.waitForNetworkIdle();
  }

  /**
   * Click login button
   */
  async clickLogin(): Promise<void> {
    await this.loginButton.click();
    await this.page.waitForURL(/\/auth\/login/);
  }

  /**
   * Click register button
   */
  async clickRegister(): Promise<void> {
    await this.registerButton.click();
    // Register redirects to API endpoint, then to auth flow
    // Wait for URL change or navigation
    await this.page.waitForLoadState('domcontentloaded', { timeout: 5000 }).catch(() => {});
  }

  /**
   * Check if hero section is visible
   */
  async isHeroVisible(): Promise<boolean> {
    try {
      await expect(this.heroSection).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if features section is visible
   */
  async areFeaturesVisible(): Promise<boolean> {
    try {
      await expect(this.featuresSection).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if CTA section is visible
   */
  async isCtaVisible(): Promise<boolean> {
    try {
      await expect(this.ctaSection).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Scroll to features section
   */
  async scrollToFeatures(): Promise<void> {
    if ((await this.featuresSection.count()) > 0) {
      await this.scrollToElement(this.featuresSection);
    }
  }

  /**
   * Scroll to CTA section
   */
  async scrollToCta(): Promise<void> {
    if ((await this.ctaSection.count()) > 0) {
      await this.scrollToElement(this.ctaSection);
    }
  }

  /**
   * Get page heading text
   */
  async getHeadingText(): Promise<string> {
    return (await this.pageHeading.textContent()) || '';
  }
}
