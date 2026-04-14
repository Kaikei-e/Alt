import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Tag Trail page (/feeds/tag-trail)
 */
export class MobileTagTrailPage extends BasePage {
	readonly feedTitle: Locator;
	readonly refreshButton: Locator;
	readonly tagButtons: Locator;
	readonly articleList: Locator;
	readonly backButton: Locator;
	readonly welcomeState: Locator;
	readonly generatingTags: Locator;
	readonly errorMessage: Locator;

	constructor(page: Page) {
		super(page);

		this.feedTitle = page.getByRole("heading").first();
		this.refreshButton = page.getByRole("button", {
			name: /new random feed/i,
		});
		this.tagButtons = page.getByRole("button").filter({
			hasNotText: /new random feed|back|open floating/i,
		});
		this.articleList = page.getByRole("list");
		this.backButton = page.getByRole("button", { name: /back/i });
		this.welcomeState = page.getByText(/start your tag trail/i);
		this.generatingTags = page.getByText(/generating tags/i);
		this.errorMessage = page.getByText(/error|failed/i).first();
	}

	get url(): string {
		return "./feeds/tag-trail";
	}

	/**
	 * Wait for feed to load.
	 */
	async waitForFeedLoaded(): Promise<void> {
		await expect(
			this.feedTitle.or(this.welcomeState).or(this.errorMessage).first(),
		).toBeVisible({ timeout: 15000 });
	}

	/**
	 * Click a tag by its text.
	 */
	async clickTag(tagText: string): Promise<void> {
		await this.page.getByRole("button", { name: tagText }).click();
	}
}
