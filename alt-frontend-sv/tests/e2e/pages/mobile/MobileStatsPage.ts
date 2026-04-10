import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Stats page
 * Alt-Paper "Circulation Ledger" editorial layout
 */
export class MobileStatsPage extends BasePage {
	readonly pageTitle: Locator;
	readonly loadingDot: Locator;

	// Ledger rows
	readonly totalFeeds: Locator;
	readonly totalArticles: Locator;
	readonly unsummarized: Locator;
	readonly unreadCount: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page
			.getByRole("heading", { name: /circulation ledger/i })
			.first();
		this.loadingDot = page.locator(".loading-pulse").first();

		this.totalFeeds = page.getByTestId("stat-total-feeds");
		this.totalArticles = page.getByTestId("stat-total-articles");
		this.unsummarized = page.getByTestId("stat-unsummarized");
		this.unreadCount = page.getByTestId("stat-unread-count");
	}

	get url(): string {
		return "./stats";
	}

	async waitForStatsLoaded(): Promise<void> {
		await expect(this.loadingDot).not.toBeVisible({ timeout: 15000 });
	}

	async getTotalFeedsValue(): Promise<string | null> {
		return this.totalFeeds.textContent();
	}

	async getTotalArticlesValue(): Promise<string | null> {
		return this.totalArticles.textContent();
	}

	async getUnreadCountValue(): Promise<string | null> {
		return this.unreadCount.textContent();
	}
}
