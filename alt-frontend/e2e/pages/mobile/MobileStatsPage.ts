import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../BasePage";

export class MobileStatsPage extends BasePage {
  readonly statsHeading: Locator;
  readonly statsCards: Locator;
  readonly connectionStatus: Locator;

  constructor(page: Page) {
    super(page);
    this.statsHeading = page.getByTestId("stats-heading");
    this.statsCards = page.getByTestId("stats-cards");
    this.connectionStatus = page.getByText(
      /Connected|Disconnected|Reconnecting/,
    );
  }

  async goto(): Promise<void> {
    await this.navigateTo("/mobile/feeds/stats");
  }

  async waitForReady(): Promise<void> {
    await expect(this.statsHeading).toBeVisible({ timeout: 15000 });
  }

  async hasStatsCards(): Promise<boolean> {
    try {
      await expect(this.statsCards).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async isConnected(): Promise<boolean> {
    const connectedText = this.page.getByText("Connected");
    try {
      await expect(connectedText).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }
}
