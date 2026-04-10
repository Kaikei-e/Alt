import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Tag Trail page (/feeds/tag-trail)
 */
export class DesktopTagTrailPage extends BasePage {
	// Header
	readonly pageTitle: Locator;

	// Feed panel (left aside)
	readonly feedTitle: Locator;
	readonly refreshButton: Locator;
	readonly tagButtons: Locator;
	readonly trailHistory: Locator;

	// Welcome state
	readonly welcomeState: Locator;

	// Loading states
	readonly generatingTags: Locator;
	readonly noTagsMessage: Locator;

	// Article grid (right panel)
	readonly articleGrid: Locator;
	readonly loadMoreButton: Locator;

	// Error state
	readonly errorMessage: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: /tag trail/i }).first();
		this.feedTitle = page.locator("aside").getByRole("heading").first();
		this.refreshButton = page.getByRole("button", {
			name: /new random feed/i,
		});
		this.tagButtons = page
			.locator("aside")
			.getByRole("button")
			.filter({
				hasNotText: /new random feed|home/i,
			});
		this.trailHistory = page
			.getByTestId("trail-breadcrumb")
			.or(page.locator('[aria-label="Trail history"]'));

		this.welcomeState = page.getByText(/start your tag trail/i);
		this.generatingTags = page.getByText(/generating tags/i);
		this.noTagsMessage = page.getByText(/no tags/i);

		this.articleGrid = page.locator("main").locator(".grid");
		this.loadMoreButton = page.getByRole("button", {
			name: /load more/i,
		});

		this.errorMessage = page.getByText(/error|failed/i).first();
	}

	get url(): string {
		return "./feeds/tag-trail";
	}

	/**
	 * Wait for the tag trail to load (feed card visible).
	 */
	async waitForFeedLoaded(): Promise<void> {
		await expect(
			this.feedTitle.or(this.welcomeState).or(this.errorMessage).first(),
		).toBeVisible({ timeout: 15000 });
	}

	/**
	 * Click a tag button by its text.
	 */
	async clickTag(tagText: string): Promise<void> {
		await this.page
			.locator("aside")
			.getByRole("button", { name: tagText })
			.click();
	}

	/**
	 * Click "Start" in the trail breadcrumb to reset.
	 */
	async clickHome(): Promise<void> {
		await this.page.getByRole("button", { name: /start|home/i }).click();
	}

	/**
	 * Get visible tag button texts.
	 */
	getTagButton(name: string): Locator {
		return this.page.locator("aside").getByRole("button", { name });
	}
}
