import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Favorites / Clippings File page (/feeds/favorites)
 */
export class DesktopFavoritesPage extends BasePage {
	readonly pageContainer: Locator;
	readonly pageTitle: Locator;
	readonly feedGrid: Locator;
	readonly loadingIndicator: Locator;
	readonly emptyState: Locator;
	readonly noMoreFeeds: Locator;

	constructor(page: Page) {
		super(page);

		this.pageContainer = page.locator('[data-role="clippings-file-page"]');
		this.pageTitle = this.pageContainer
			.getByRole("heading", { name: /the clippings file/i })
			.first();
		this.feedGrid = this.pageContainer.locator(".grid");
		this.loadingIndicator = this.pageContainer
			.locator(".loading-pulse")
			.first();
		this.emptyState = page.getByText(/no clippings yet/i);
		this.noMoreFeeds = page.getByText(/end of wire/i);
	}

	get url(): string {
		return "./feeds/favorites";
	}
}
