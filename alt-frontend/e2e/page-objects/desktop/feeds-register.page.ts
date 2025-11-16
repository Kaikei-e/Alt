import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

/**
 * Desktop Feed Register Page Object
 * Represents the /desktop/feeds/register page
 */
export class DesktopFeedRegisterPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly feedUrlInput: Locator;
  readonly feedTitleInput: Locator;
  readonly feedDescriptionInput: Locator;
  readonly categorySelect: Locator;
  readonly submitButton: Locator;
  readonly cancelButton: Locator;
  readonly errorMessage: Locator;
  readonly successMessage: Locator;
  readonly validationMessage: Locator;
  readonly validateButton: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole("heading", {
      name: /register feed|add feed|new feed/i,
    });
    this.feedUrlInput = page.getByLabel(/url|feed url|rss url/i);
    this.feedTitleInput = page.getByLabel(/title|feed title|name/i);
    this.feedDescriptionInput = page.getByLabel(/description/i);
    this.categorySelect = page.getByLabel(/category/i);
    this.submitButton = page.getByRole("button", {
      name: /submit|add|register|save/i,
    });
    this.cancelButton = page.getByRole("button", { name: /cancel|back/i });
    this.errorMessage = page.getByRole("alert");
    this.successMessage = page.getByRole("status");
    this.validationMessage = page.locator('[data-testid="validation-message"]');
    this.validateButton = page.getByRole("button", { name: /validate|check/i });
  }

  /**
   * Navigate to feed register page
   */
  async goto(): Promise<void> {
    await this.page.goto("/desktop/feeds/register");
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await expect(this.feedUrlInput).toBeVisible();
    await expect(this.submitButton).toBeVisible();
  }

  /**
   * Register a feed with URL
   */
  async registerFeed(
    url: string,
    title?: string,
    description?: string,
    category?: string,
  ): Promise<void> {
    await this.feedUrlInput.fill(url);

    if (title && (await this.feedTitleInput.count()) > 0) {
      await this.feedTitleInput.fill(title);
    }

    if (description && (await this.feedDescriptionInput.count()) > 0) {
      await this.feedDescriptionInput.fill(description);
    }

    if (category && (await this.categorySelect.count()) > 0) {
      await this.categorySelect.selectOption(category);
    }

    await this.submitButton.click();
  }

  /**
   * Fill feed URL
   */
  async fillUrl(url: string): Promise<void> {
    await this.feedUrlInput.fill(url);
  }

  /**
   * Fill feed title
   */
  async fillTitle(title: string): Promise<void> {
    if ((await this.feedTitleInput.count()) > 0) {
      await this.feedTitleInput.fill(title);
    }
  }

  /**
   * Fill feed description
   */
  async fillDescription(description: string): Promise<void> {
    if ((await this.feedDescriptionInput.count()) > 0) {
      await this.feedDescriptionInput.fill(description);
    }
  }

  /**
   * Select category
   */
  async selectCategory(category: string): Promise<void> {
    if ((await this.categorySelect.count()) > 0) {
      await this.categorySelect.selectOption(category);
    }
  }

  /**
   * Validate feed URL
   */
  async validateUrl(): Promise<void> {
    if ((await this.validateButton.count()) > 0) {
      await this.validateButton.click();
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Click submit button
   */
  async clickSubmit(): Promise<void> {
    await this.submitButton.click();
  }

  /**
   * Click cancel button
   */
  async clickCancel(): Promise<void> {
    await this.cancelButton.click();
    await this.page.waitForURL(/\/desktop\/feeds$/);
  }

  /**
   * Get error message
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
   * Get success message
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
   * Wait for successful registration
   */
  async waitForSuccess(): Promise<void> {
    // Wait for redirect to feeds page or success message
    try {
      await this.page.waitForURL(/\/desktop\/feeds$/, { timeout: 10000 });
    } catch {
      // If no redirect, check for success message
      await expect(this.successMessage).toBeVisible({ timeout: 5000 });
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
   * Clear form
   */
  async clearForm(): Promise<void> {
    await this.feedUrlInput.clear();

    if ((await this.feedTitleInput.count()) > 0) {
      await this.feedTitleInput.clear();
    }

    if ((await this.feedDescriptionInput.count()) > 0) {
      await this.feedDescriptionInput.clear();
    }
  }
}
