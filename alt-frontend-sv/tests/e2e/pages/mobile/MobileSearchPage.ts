import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Search page (/feeds/search)
 */
export class MobileSearchPage extends BasePage {
	readonly searchInput: Locator;
	readonly searchButton: Locator;
	readonly resultItems: Locator;
	readonly emptyState: Locator;
	readonly validationError: Locator;
	readonly resultCount: Locator;
	readonly infiniteScrollSentinel: Locator;
	readonly noMoreResults: Locator;

	constructor(page: Page) {
		super(page);

		this.searchInput = page.getByTestId("search-input");
		this.searchButton = page.getByRole("button", { name: "Search" });
		this.resultItems = page.getByTestId("search-result-item");
		this.emptyState = page.getByText(/no results/i);
		this.validationError = page.getByText(
			"Search query must be at least 2 characters",
		);
		this.resultCount = page.getByText(/search results/i);
		this.infiniteScrollSentinel = page.getByTestId("infinite-scroll-sentinel");
		this.noMoreResults = page.getByText("No more results to load");
	}

	get url(): string {
		return "./feeds/search";
	}

	/**
	 * Type a search query using sequential key presses (for Svelte reactivity).
	 */
	async typeQuery(query: string): Promise<void> {
		await this.searchInput.click();
		await this.searchInput.pressSequentially(query, { delay: 50 });
	}

	/**
	 * Submit the search.
	 */
	async submitSearch(): Promise<void> {
		await expect(this.searchButton).toBeEnabled();
		await this.searchButton.click();
	}

	/**
	 * Perform a complete search.
	 */
	async search(query: string): Promise<void> {
		await this.typeQuery(query);
		await this.submitSearch();
	}

	/**
	 * Trigger infinite scroll by scrolling to the bottom of the page.
	 * Uses evaluate-based scroll instead of scrollIntoViewIfNeeded on the
	 * sentinel, because the sentinel is conditionally rendered ({#if hasMore})
	 * and may already be removed from the DOM on small viewports.
	 */
	async triggerLoadMore(): Promise<void> {
		await this.page.evaluate(() =>
			window.scrollTo(0, document.body.scrollHeight),
		);
	}
}
