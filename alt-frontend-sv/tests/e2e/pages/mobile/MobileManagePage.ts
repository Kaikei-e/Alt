import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Feed Management page (/settings/feeds)
 */
export class MobileManagePage extends BasePage {
	readonly pageTitle: Locator;
	readonly addFeedButton: Locator;
	readonly feedUrlInput: Locator;
	readonly submitButton: Locator;
	readonly cancelButton: Locator;
	readonly feedList: Locator;
	readonly validationError: Locator;
	readonly refreshButton: Locator;
	readonly emptyState: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByText("Feed Management");
		this.addFeedButton = page.getByRole("button", {
			name: "Add a new feed",
		});
		this.feedUrlInput = page.getByPlaceholder("https://example.com/feed.xml");
		this.submitButton = page.getByRole("button", { name: "Add feed" });
		this.cancelButton = page.getByRole("button", { name: "Cancel" });
		this.feedList = page.getByRole("list");
		this.validationError = page.getByText("Please enter the RSS URL.", {
			exact: true,
		});
		this.refreshButton = page.getByLabel("Refresh");
		this.emptyState = page.getByText("No feeds registered yet.");
	}

	get url(): string {
		return "./settings/feeds";
	}

	/**
	 * Open the add feed form.
	 */
	async openAddFeedForm(): Promise<void> {
		await this.addFeedButton.click();
		await expect(this.feedUrlInput).toBeVisible();
	}

	/**
	 * Add a feed URL.
	 */
	async addFeed(url: string): Promise<void> {
		await this.feedUrlInput.fill(url);
		await this.submitButton.click();
	}

	/**
	 * Submit empty form to trigger validation.
	 */
	async submitEmptyForm(): Promise<void> {
		await this.submitButton.click();
	}

	/**
	 * Get delete buttons for feed links.
	 */
	getDeleteButtons(): Locator {
		return this.page.getByLabel("Delete feed link");
	}
}
