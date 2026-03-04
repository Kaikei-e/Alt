import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Viewed/History page (/feeds/viewed)
 */
export class DesktopViewedPage extends BasePage {
	readonly pageTitle: Locator;
	readonly feedGrid: Locator;
	readonly loadingSpinner: Locator;
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

		this.pageTitle = page
			.getByRole("heading", { name: /read history/i })
			.first();
		this.feedGrid = page.locator(".grid");
		this.loadingSpinner = page.locator(".animate-spin").first();
		this.emptyState = page.getByText("No viewed feeds yet");
		this.errorMessage = page.getByText(/error/i).first();
		this.noMoreFeeds = page.getByText(/no more feeds/i);

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
		await expect(this.loadingSpinner).not.toBeVisible({ timeout: 15000 });
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
