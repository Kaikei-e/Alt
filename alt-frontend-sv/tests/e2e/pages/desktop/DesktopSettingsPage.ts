import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Settings page (/settings/feeds)
 */
export class DesktopSettingsPage extends BasePage {
	readonly pageTitle: Locator;
	readonly loadingSpinner: Locator;

	// Feed management
	readonly feedList: Locator;
	readonly addFeedInput: Locator;
	readonly addFeedButton: Locator;
	readonly emptyState: Locator;

	// Dialogs
	readonly confirmDialog: Locator;
	readonly confirmButton: Locator;
	readonly cancelButton: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: /settings|feeds/i }).first();
		this.loadingSpinner = page.locator(".animate-spin").first();

		this.feedList = page.locator("[data-testid='feed-list']");
		this.addFeedInput = page.getByPlaceholder(/url|feed/i);
		this.addFeedButton = page.getByRole("button", { name: /add|register/i });
		this.emptyState = page.getByText(/no feeds registered/i);

		this.confirmDialog = page.locator('[role="alertdialog"]');
		this.confirmButton = page.getByRole("button", { name: /confirm|delete|yes/i });
		this.cancelButton = page.getByRole("button", { name: /cancel|no/i });
	}

	get url(): string {
		return "./settings/feeds";
	}

	async waitForSettingsLoaded(): Promise<void> {
		await expect(this.loadingSpinner).not.toBeVisible({ timeout: 15000 });
	}

	getFeedItems(): Locator {
		return this.feedList.locator("[data-testid='feed-item']");
	}

	getFeedByUrl(url: string): Locator {
		return this.feedList.locator(`text=${url}`);
	}

	async addFeed(url: string): Promise<void> {
		await this.addFeedInput.fill(url);
		await this.addFeedButton.click();
	}

	async deleteFeed(url: string): Promise<void> {
		const feedItem = this.getFeedByUrl(url);
		const deleteButton = feedItem.locator("..").getByRole("button", { name: /delete|remove/i });
		await deleteButton.click();
		await expect(this.confirmDialog).toBeVisible();
		await this.confirmButton.click();
	}
}
