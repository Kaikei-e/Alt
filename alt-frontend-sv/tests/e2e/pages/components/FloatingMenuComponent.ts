import type { Locator, Page } from "@playwright/test";

/**
 * Component POM for the Mobile Floating Menu (bottom sheet navigation).
 */
export class FloatingMenuComponent {
	readonly page: Page;
	readonly menuTrigger: Locator;
	readonly sheet: Locator;
	readonly closeButton: Locator;

	constructor(page: Page) {
		this.page = page;
		this.menuTrigger = page.getByLabel("Open floating menu");
		this.sheet = page.locator('[role="dialog"]');
		this.closeButton = page.getByLabel("Close dialog");
	}

	/**
	 * Open the floating menu.
	 */
	async open(): Promise<void> {
		await this.menuTrigger.click();
	}

	/**
	 * Close the floating menu.
	 */
	async close(): Promise<void> {
		await this.closeButton.click();
	}

	/**
	 * Navigate to a menu item by label text.
	 */
	async navigateTo(label: string | RegExp): Promise<void> {
		await this.sheet.getByRole("link", { name: label }).click();
	}

	/**
	 * Get a menu link by label.
	 */
	getLink(label: string | RegExp): Locator {
		return this.sheet.getByRole("link", { name: label });
	}
}
