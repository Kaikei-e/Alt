import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Feeds page
 */
export class MobileFeedsPage extends BasePage {
	readonly pageTitle: Locator;
	readonly feedList: Locator;
	readonly loadingSpinner: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;
	readonly pullToRefresh: Locator;

	// Navigation
	readonly bottomNav: Locator;
	readonly swipeButton: Locator;
	readonly searchButton: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: /feeds/i }).first();
		this.feedList = page.locator("[data-testid='feed-list']");
		this.loadingSpinner = page.locator(".animate-spin").first();
		this.emptyState = page.getByText(/no feeds/i);
		this.errorMessage = page.getByText(/error/i);
		this.pullToRefresh = page.getByTestId("pull-to-refresh");

		this.bottomNav = page.locator("nav").last();
		this.swipeButton = page.getByRole("link", { name: /swipe/i });
		this.searchButton = page.getByRole("button", { name: /search/i });
	}

	get url(): string {
		return "./feeds";
	}

	async waitForFeedsLoaded(): Promise<void> {
		await expect(this.loadingSpinner).not.toBeVisible({ timeout: 15000 });
		await expect(this.feedList.or(this.emptyState).first()).toBeVisible();
	}

	getFeedCards(): Locator {
		return this.feedList.locator("button, a").filter({ hasText: /.+/ });
	}

	async getFeedCount(): Promise<number> {
		return this.getFeedCards().count();
	}

	async selectFeed(title: string): Promise<void> {
		await this.page.getByText(title).click();
	}

	async navigateToSwipe(): Promise<void> {
		await this.swipeButton.click();
	}
}
