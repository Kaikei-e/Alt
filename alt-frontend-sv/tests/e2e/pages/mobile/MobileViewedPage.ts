import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Viewed/History page (/feeds/viewed)
 */
export class MobileViewedPage extends BasePage {
	readonly emptyState: Locator;
	readonly emptyIcon: Locator;

	constructor(page: Page) {
		super(page);

		this.emptyState = page.getByText("No History Yet");
		this.emptyIcon = page.getByTestId("empty-viewed-feeds-icon");
	}

	get url(): string {
		return "./feeds/viewed";
	}
}
