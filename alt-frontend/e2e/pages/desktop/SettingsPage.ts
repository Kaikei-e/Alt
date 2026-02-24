import { type Locator, type Page, expect } from "@playwright/test";
import { BasePage } from "../BasePage";

export class SettingsPage extends BasePage {
  readonly settingsPage: Locator;
  readonly settingsHeading: Locator;
  readonly settingsForm: Locator;
  readonly nameInput: Locator;
  readonly saveButton: Locator;
  readonly successMessage: Locator;
  readonly errorMessage: Locator;

  constructor(page: Page) {
    super(page);
    this.settingsPage = page.getByTestId("settings-page");
    this.settingsHeading = page.getByTestId("settings-heading");
    this.settingsForm = page.getByTestId("settings-form");
    this.nameInput = page.getByTestId("settings-name-input");
    this.saveButton = page.getByTestId("settings-save-button");
    this.successMessage = page.getByTestId("settings-success-message");
    this.errorMessage = page.getByTestId("settings-error-message");
  }

  async goto(): Promise<void> {
    await this.navigateTo("/desktop/settings");
  }

  async waitForReady(): Promise<void> {
    await expect(this.settingsPage).toBeVisible({ timeout: 15000 });
    await expect(this.nameInput).toBeVisible();
  }

  async updateName(name: string): Promise<void> {
    await this.nameInput.clear();
    await this.nameInput.fill(name);
  }

  async saveChanges(): Promise<void> {
    await this.saveButton.click();
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

  async getNameValue(): Promise<string> {
    return await this.nameInput.inputValue();
  }
}
