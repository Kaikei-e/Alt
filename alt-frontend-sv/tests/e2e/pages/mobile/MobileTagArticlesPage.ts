import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Tag Articles page (/articles/by-tag?tag=...)
 */
export class MobileTagArticlesPage extends BasePage {
	readonly pageTitle: Locator;
	readonly articleList: Locator;
	readonly loadMoreButton: Locator;
	readonly emptyState: Locator;
	readonly backButton: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { level: 1 });
		this.articleList = page.getByTestId("article-list");
		this.loadMoreButton = page.getByRole("button", { name: /load more/i });
		this.emptyState = page.getByTestId("empty-state");
		this.backButton = page.getByRole("link", { name: /back|home/i }).first();
	}

	get url(): string {
		return "./articles/by-tag?tag=AI";
	}

	async gotoWithTag(tagName: string): Promise<void> {
		await this.page.goto(
			`./articles/by-tag?tag=${encodeURIComponent(tagName)}`,
		);
	}

	async waitForArticlesLoaded(): Promise<void> {
		await this.page.waitForLoadState("domcontentloaded");
		await this.articleList
			.or(this.emptyState)
			.first()
			.waitFor({ timeout: 15000 });
	}
}
