import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Feeds page (/desktop/feeds)
 */
export class DesktopFeedsPage extends BasePage {
	// Page header
	readonly pageTitle: Locator;

	// Feed grid
	readonly feedGrid: Locator;
	readonly loadingSpinner: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;

	// Filters
	readonly unreadOnlyCheckbox: Locator;
	readonly sortByDropdown: Locator;

	// Feed detail modal
	readonly feedDetailModal: Locator;
	readonly modalTitle: Locator;
	readonly modalCloseButton: Locator;
	readonly markAsReadButton: Locator;
	readonly fullArticleButton: Locator;
	readonly summarizeButton: Locator;

	// Modal navigation arrows
	readonly prevFeedButton: Locator;
	readonly nextFeedButton: Locator;

	// Infinite scroll
	readonly loadMoreTrigger: Locator;
	readonly loadingMoreSpinner: Locator;
	readonly noMoreFeedsText: Locator;

	constructor(page: Page) {
		super(page);

		// Page elements
		this.pageTitle = page.getByRole("heading", { name: /feeds/i }).first();
		this.feedGrid = page.locator(".grid");
		this.loadingSpinner = page.locator(".animate-spin").first();
		this.emptyState = page.getByText("No feeds found");
		this.errorMessage = page.getByText(/error loading feeds/i);

		// Filters (role-based locators)
		this.unreadOnlyCheckbox = page.getByRole("checkbox", { name: /unread/i });
		this.sortByDropdown = page.getByRole("combobox");

		// Feed detail modal
		this.feedDetailModal = page.locator('[role="dialog"]');
		this.modalTitle = this.feedDetailModal.locator("h2");
		// Dialog.Close button - there are 2 close buttons (footer and X icon), use the one with text "Close"
		this.modalCloseButton = this.feedDetailModal
			.getByRole("button", { name: /^Close$/i })
			.first();
		this.markAsReadButton = page.getByRole("button", { name: /mark as read/i });
		this.fullArticleButton = page.getByRole("button", {
			name: /full article/i,
		});
		this.summarizeButton = page.getByRole("button", { name: /summarize/i });

		// Modal navigation arrows
		this.prevFeedButton = this.feedDetailModal.getByRole("button", {
			name: /previous feed/i,
		});
		this.nextFeedButton = this.feedDetailModal.getByRole("button", {
			name: /next feed/i,
		});

		// Infinite scroll elements
		this.loadMoreTrigger = page.getByText("Scroll for more");
		this.loadingMoreSpinner = page.locator(".animate-spin").last();
		this.noMoreFeedsText = page.getByText("No more feeds");
	}

	get url(): string {
		return "./desktop/feeds";
	}

	/**
	 * Get all visible feed cards
	 */
	getFeedCards(): Locator {
		return this.page.locator('button[aria-label^="Open"]');
	}

	/**
	 * Get a specific feed card by title
	 */
	getFeedCardByTitle(title: string): Locator {
		return this.page.getByRole("button", {
			name: new RegExp(`Open ${title}`, "i"),
		});
	}

	/**
	 * Wait for feeds to load (loading spinner disappears and grid is visible)
	 */
	async waitForFeedsLoaded(): Promise<void> {
		await expect(this.loadingSpinner).not.toBeVisible({ timeout: 15000 });
		// Wait for either feeds to appear or empty state
		await expect(this.feedGrid.or(this.emptyState).first()).toBeVisible({
			timeout: 10000,
		});
	}

	/**
	 * Click on a feed card to open the detail modal
	 */
	async selectFeed(title: string): Promise<void> {
		const card = this.getFeedCardByTitle(title);
		await card.click();
		await expect(this.feedDetailModal).toBeVisible();
	}

	/**
	 * Close the feed detail modal
	 */
	async closeModal(): Promise<void> {
		await this.modalCloseButton.click();
		await expect(this.feedDetailModal).not.toBeVisible();
	}

	/**
	 * Mark the currently open feed as read
	 * Note: After marking as read, modal navigates to next feed or closes if last
	 */
	async markCurrentFeedAsRead(): Promise<void> {
		await this.markAsReadButton.click();
	}

	/**
	 * Toggle unread only filter
	 */
	async toggleUnreadOnly(): Promise<void> {
		await this.unreadOnlyCheckbox.click();
	}

	/**
	 * Get the count of visible feed cards
	 */
	async getFeedCount(): Promise<number> {
		return this.getFeedCards().count();
	}

	/**
	 * Assert that the modal shows the correct feed title
	 */
	async expectModalTitle(title: string): Promise<void> {
		await expect(this.modalTitle).toContainText(title);
	}

	/**
	 * Navigate to the next feed in the modal
	 */
	async navigateToNextFeed(): Promise<void> {
		await this.nextFeedButton.click();
	}

	/**
	 * Navigate to the previous feed in the modal
	 */
	async navigateToPreviousFeed(): Promise<void> {
		await this.prevFeedButton.click();
	}

	/**
	 * Navigate to next feed using keyboard
	 */
	async navigateToNextFeedWithKeyboard(): Promise<void> {
		await this.page.keyboard.press("ArrowRight");
	}

	/**
	 * Navigate to previous feed using keyboard
	 */
	async navigateToPreviousFeedWithKeyboard(): Promise<void> {
		await this.page.keyboard.press("ArrowLeft");
	}
}
