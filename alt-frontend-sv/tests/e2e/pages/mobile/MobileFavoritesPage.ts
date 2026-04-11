import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Favorites / Clippings File page (/feeds/favorites)
 */
export class MobileFavoritesPage extends BasePage {
	readonly pageContainer: Locator;
	readonly emptyState: Locator;
	readonly feedList: Locator;
	readonly loadingIndicator: Locator;

	constructor(page: Page) {
		super(page);

		this.pageContainer = page.locator('[data-role="clippings-file-page"]');
		this.emptyState = page.getByText(/no clippings yet/i);
		this.feedList = page.locator('[data-role="clippings-feed-list"]');
		this.loadingIndicator = page.locator(".loading-pulse").first();
	}

	get url(): string {
		return "./feeds/favorites";
	}
}
