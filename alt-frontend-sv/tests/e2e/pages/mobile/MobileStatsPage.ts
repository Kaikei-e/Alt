import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Stats page
 */
export class MobileStatsPage extends BasePage {
	readonly pageTitle: Locator;
	readonly loadingSpinner: Locator;

	// Stats cards
	readonly totalFeeds: Locator;
	readonly totalArticles: Locator;
	readonly unreadCount: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: /stats|statistics/i }).first();
		this.loadingSpinner = page.locator(".animate-spin").first();

		this.totalFeeds = page.getByTestId("stat-total-feeds");
		this.totalArticles = page.getByTestId("stat-total-articles");
		this.unreadCount = page.getByTestId("stat-unread-count");
	}

	get url(): string {
		return "./feeds/stats";
	}

	async waitForStatsLoaded(): Promise<void> {
		await expect(this.loadingSpinner).not.toBeVisible({ timeout: 15000 });
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
