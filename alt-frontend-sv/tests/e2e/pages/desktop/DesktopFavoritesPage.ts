import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Favorites page (/feeds/favorites)
 */
export class DesktopFavoritesPage extends BasePage {
	readonly pageTitle: Locator;
	readonly feedGrid: Locator;
	readonly loadingSpinner: Locator;
	readonly emptyState: Locator;
	readonly noMoreFeeds: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: /favorites/i }).first();
		this.feedGrid = page.locator(".grid");
		this.loadingSpinner = page.locator(".animate-spin").first();
		this.emptyState = page.getByText(/no feeds/i);
		this.noMoreFeeds = page.getByText(/no more feeds/i);
	}

	get url(): string {
		return "./feeds/favorites";
	}
}
