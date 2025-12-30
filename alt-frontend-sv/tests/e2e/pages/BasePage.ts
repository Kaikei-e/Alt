import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";

/**
 * Base Page Object class for all page objects.
 * Provides common functionality for navigation and assertions.
 */
export abstract class BasePage {
	readonly page: Page;

	constructor(page: Page) {
		this.page = page;
	}

	/**
	 * The URL path for this page (relative to baseURL).
	 */
	abstract get url(): string;

	/**
	 * Navigate to this page.
	 */
	async goto(): Promise<void> {
		await this.page.goto(this.url);
	}

	/**
	 * Wait for the page to fully load.
	 */
	async waitForLoad(): Promise<void> {
		await this.page.waitForLoadState("networkidle");
	}

	/**
	 * Assert that a locator is visible.
	 */
	async expectVisible(locator: Locator): Promise<void> {
		await expect(locator).toBeVisible();
	}

	/**
	 * Assert that a locator is not visible.
	 */
	async expectNotVisible(locator: Locator): Promise<void> {
		await expect(locator).not.toBeVisible();
	}

	/**
	 * Assert that a locator has specific text.
	 */
	async expectText(locator: Locator, text: string | RegExp): Promise<void> {
		await expect(locator).toContainText(text);
	}

	/**
	 * Wait for a loading spinner to disappear.
	 */
	async waitForLoadingComplete(
		spinnerSelector = ".animate-spin",
		timeout = 15000,
	): Promise<void> {
		const spinner = this.page.locator(spinnerSelector);
		await expect(spinner).not.toBeVisible({ timeout });
	}
}
