import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Search page (/feeds with search)
 */
export class DesktopSearchPage extends BasePage {
	readonly searchInput: Locator;
	readonly searchButton: Locator;
	readonly loadingSpinner: Locator;
	readonly resultsList: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;

	constructor(page: Page) {
		super(page);

		this.searchInput = page.getByPlaceholder(/search/i);
		this.searchButton = page.getByRole("button", { name: /search/i });
		this.loadingSpinner = page.locator(".animate-spin").first();
		this.resultsList = page.locator("[data-testid='search-results']");
		this.emptyState = page.getByText(/no results/i);
		this.errorMessage = page.getByText(/error/i);
	}

	get url(): string {
		return "./feeds";
	}

	async search(query: string): Promise<void> {
		await this.searchInput.fill(query);
		await this.searchButton.click();
	}

	async waitForResults(): Promise<void> {
		await expect(this.loadingSpinner).not.toBeVisible({ timeout: 15000 });
		await expect(this.resultsList.or(this.emptyState).first()).toBeVisible();
	}

	getResultItems(): Locator {
		return this.resultsList.locator("button, a").filter({ hasText: /.+/ });
	}

	getResultByTitle(title: string): Locator {
		return this.resultsList.getByText(title);
	}

	async getResultCount(): Promise<number> {
		return this.getResultItems().count();
	}
}
