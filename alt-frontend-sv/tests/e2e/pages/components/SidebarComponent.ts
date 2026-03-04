import type { Locator, Page } from "@playwright/test";

/**
 * Component POM for the Desktop Sidebar navigation.
 */
export class SidebarComponent {
	readonly page: Page;
	readonly root: Locator;
	readonly brandTitle: Locator;
	readonly nav: Locator;

	constructor(page: Page) {
		this.page = page;
		this.root = page.locator("aside");
		this.brandTitle = this.root.getByRole("heading", { name: /Alt Reader/i });
		this.nav = this.root.getByRole("navigation");
	}

	/**
	 * Get a nav link by its label text.
	 */
	getLink(label: string | RegExp): Locator {
		return this.nav.getByRole("link", { name: label });
	}

	/**
	 * Navigate to a sidebar link by label.
	 */
	async navigateTo(label: string | RegExp): Promise<void> {
		await this.getLink(label).click();
	}

	/**
	 * Check if a link is active (has active CSS class).
	 */
	isLinkActive(label: string | RegExp): Locator {
		return this.getLink(label);
	}

	/**
	 * Toggle a collapsible section by its heading text.
	 */
	async toggleSection(label: string | RegExp): Promise<void> {
		const section = this.root.getByRole("button", { name: label });
		await section.click();
	}

	/**
	 * Get all navigation links.
	 */
	getAllLinks(): Locator {
		return this.nav.getByRole("link");
	}
}
