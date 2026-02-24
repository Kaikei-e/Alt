import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../BasePage";

export class MobileRegisterFeedPage extends BasePage {
  readonly registerFeedPage: Locator;
  readonly registerFeedHeading: Locator;
  readonly feedUrlInput: Locator;
  readonly feedUrlError: Locator;
  readonly registerButton: Locator;
  readonly successMessage: Locator;
  readonly errorMessage: Locator;

  constructor(page: Page) {
    super(page);
    this.registerFeedPage = page.getByTestId("register-feed-page");
    this.registerFeedHeading = page.getByTestId("register-feed-heading");
    this.feedUrlInput = page.getByTestId("feed-url-input");
    this.feedUrlError = page.getByTestId("feed-url-error");
    this.registerButton = page.getByTestId("register-feed-button");
    this.successMessage = page.getByTestId("register-success-message");
    this.errorMessage = page.getByTestId("register-error-message");
  }

  async goto(): Promise<void> {
    await this.navigateTo("/mobile/feeds/register");
  }

  async waitForReady(): Promise<void> {
    await expect(this.registerFeedPage).toBeVisible({ timeout: 15000 });
    await expect(this.feedUrlInput).toBeVisible();
  }

  async enterFeedUrl(url: string): Promise<void> {
    await this.feedUrlInput.fill(url);
  }

  async submitForm(): Promise<void> {
    await this.registerButton.click();
  }

  async registerFeed(url: string): Promise<void> {
    await this.enterFeedUrl(url);
    await this.submitForm();
  }

  async hasValidationError(): Promise<boolean> {
    try {
      await expect(this.feedUrlError).toBeVisible({ timeout: 3000 });
      return true;
    } catch {
      return false;
    }
  }

  async hasSuccessMessage(): Promise<boolean> {
    try {
      await expect(this.successMessage).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async hasErrorMessage(): Promise<boolean> {
    try {
      await expect(this.errorMessage).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async clearInput(): Promise<void> {
    await this.feedUrlInput.clear();
  }
}
