import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Viewed/Morgue Desk page (/feeds/viewed)
 */
export class DesktopViewedPage extends BasePage {
	readonly pageContainer: Locator;
	readonly pageTitle: Locator;
	readonly feedGrid: Locator;
	readonly loadingIndicator: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;
	readonly noMoreFeeds: Locator;

	// Feed detail modal
	readonly feedDetailModal: Locator;
	readonly modalCloseButton: Locator;
	readonly prevFeedButton: Locator;
	readonly nextFeedButton: Locator;

	constructor(page: Page) {
		super(page);

		this.pageContainer = page.locator('[data-role="morgue-desk-page"]');
		this.pageTitle = this.pageContainer
			.getByRole("heading", { name: /the morgue desk/i })
			.first();
		this.feedGrid = this.pageContainer.locator(".grid");
		this.loadingIndicator = this.pageContainer
			.locator(".loading-pulse")
			.first();
		this.emptyState = page.getByText(/nothing filed yet/i);
		this.errorMessage = page.getByText(/error/i).first();
		this.noMoreFeeds = page.getByText(/end of wire/i);

		this.feedDetailModal = page.locator('[role="dialog"]');
		this.modalCloseButton = this.feedDetailModal
			.getByRole("button", { name: /^close$/i })
			.first();
		this.prevFeedButton = this.feedDetailModal.getByRole("button", {
			name: /previous feed/i,
		});
		this.nextFeedButton = this.feedDetailModal.getByRole("button", {
			name: /next feed/i,
		});
	}

	get url(): string {
		return "./feeds/viewed";
	}

	/**
	 * Wait for viewed feeds to load.
	 */
	async waitForFeedsLoaded(): Promise<void> {
		await expect(this.loadingIndicator).not.toBeVisible({ timeout: 15000 });
		await expect(this.feedGrid.or(this.emptyState).first()).toBeVisible({
			timeout: 10000,
		});
	}

	/**
	 * Get all visible feed cards.
	 */
	getFeedCards(): Locator {
		return this.page.locator('button[aria-label^="Open"]');
	}

	/**
	 * Click a feed card to open modal.
	 */
	async selectFeed(title: string): Promise<void> {
		await this.page
			.getByRole("button", { name: new RegExp(`Open ${title}`, "i") })
			.click();
		await expect(this.feedDetailModal).toBeVisible();
	}

	/**
	 * Close the feed detail modal.
	 */
	async closeModal(): Promise<void> {
		await this.modalCloseButton.click();
		await expect(this.feedDetailModal).not.toBeVisible();
	}
}
