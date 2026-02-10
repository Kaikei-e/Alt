import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Recap page (/desktop/recap)
 */
export class DesktopRecapPage extends BasePage {
	// Page header
	readonly pageTitle: Locator;

	// Loading and states
	readonly loadingSpinner: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;

	// Genre list (left column)
	readonly genreList: Locator;

	// Recap detail (right column)
	readonly recapDetail: Locator;

	constructor(page: Page) {
		super(page);

		// Page elements - PageHeader has static title "Recap"
		this.pageTitle = page.getByRole("heading", { name: /Recap/i });
		this.loadingSpinner = page.locator(".animate-spin").first();
		this.emptyState = page.getByText("No recap data available");
		this.errorMessage = page.getByText(/No recap data available yet/i);

		// Layout sections
		this.genreList = page.locator(".col-span-1");
		this.recapDetail = page.locator(".col-span-2");
	}

	get url(): string {
		return "./recap";
	}

	/**
	 * Get all genre buttons/items in the list
	 */
	getGenreItems(): Locator {
		return this.page.getByRole("button").filter({ hasText: /.+/ });
	}

	/**
	 * Get a specific genre by name
	 */
	getGenreByName(name: string): Locator {
		return this.page.getByRole("button", { name: new RegExp(name, "i") });
	}

	/**
	 * Wait for recap data to load
	 */
	async waitForRecapLoaded(): Promise<void> {
		await expect(this.loadingSpinner).not.toBeVisible({ timeout: 15000 });
	}

	/**
	 * Select a genre from the list
	 */
	async selectGenre(name: string): Promise<void> {
		const genreButton = this.getGenreByName(name);
		await genreButton.click();
	}

	/**
	 * Check if a genre is currently selected (has active state)
	 */
	async isGenreSelected(name: string): Promise<boolean> {
		const genreButton = this.getGenreByName(name);
		// Check for active/selected styling class
		const classList = await genreButton.getAttribute("class");
		return classList?.includes("bg-") ?? false;
	}

	/**
	 * Get recap detail heading for the selected genre
	 */
	getRecapDetailHeading(): Locator {
		return this.recapDetail.getByRole("heading").first();
	}

	/**
	 * Get cluster items in the recap detail
	 */
	getClusterItems(): Locator {
		return this.recapDetail
			.locator("[class*='border']")
			.filter({ hasText: /.+/ });
	}
}
