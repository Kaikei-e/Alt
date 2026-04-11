import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Viewed/Morgue Desk page (/feeds/viewed)
 */
export class MobileViewedPage extends BasePage {
	readonly emptyState: Locator;
	readonly emptyRegion: Locator;
	readonly feedList: Locator;

	constructor(page: Page) {
		super(page);

		this.emptyState = page.getByText(/nothing filed yet/i);
		this.emptyRegion = page.locator('[data-role="morgue-empty-state"]');
		this.feedList = page.locator('[data-role="morgue-feed-list"]');
	}

	get url(): string {
		return "./feeds/viewed";
	}
}
