import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile 3-Day Recap page (/recap?window=3)
 */
export class Mobile3DayRecapPage extends BasePage {
	readonly skeletonContainer: Locator;
	readonly errorMessage: Locator;
	readonly retryButton: Locator;
	readonly emptyState: Locator;

	constructor(page: Page) {
		super(page);

		this.skeletonContainer = page.getByTestId("recap-skeleton-container");
		this.errorMessage = page.getByText("Error loading recap");
		this.retryButton = page.getByRole("button", { name: /retry/i });
		this.emptyState = page.getByText("No Recap Yet");
	}

	get url(): string {
		return "./recap?window=3";
	}

	/**
	 * Wait for recap content to load (skeleton disappears).
	 */
	async waitForRecapLoaded(): Promise<void> {
		await expect(this.skeletonContainer).not.toBeVisible({
			timeout: 15000,
		});
	}
}
