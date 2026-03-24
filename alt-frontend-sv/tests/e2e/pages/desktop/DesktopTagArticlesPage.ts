import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Tag Articles page (/articles/by-tag?tag=...)
 */
export class DesktopTagArticlesPage extends BasePage {
	readonly pageTitle: Locator;
	readonly articleList: Locator;
	readonly articleGrid: Locator;
	readonly loadMoreButton: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;
	readonly backLink: Locator;
	readonly detailPanel: Locator;
	readonly fetchContentButton: Locator;
	readonly closeDetailButton: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { level: 1 });
		this.articleList = page.getByTestId("article-list");
		this.articleGrid = page.getByTestId("article-grid");
		this.loadMoreButton = page.getByRole("button", { name: /load more/i });
		this.emptyState = page.getByTestId("empty-state");
		this.errorMessage = page.getByText(/failed|error/i).first();
		this.backLink = page.getByRole("link", { name: /home/i }).first();
		this.detailPanel = page.getByTestId("article-detail-panel");
		this.fetchContentButton = page.getByRole("button", {
			name: /fetch content/i,
		});
		this.closeDetailButton = page
			.getByTestId("article-detail-panel")
			.getByRole("button", { name: /close/i });
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
		// Wait for either article list or empty state
		await this.articleList
			.or(this.emptyState)
			.or(this.errorMessage)
			.first()
			.waitFor({ timeout: 15000 });
	}

	getArticle(id: string): Locator {
		return this.page.getByTestId(`tag-article-${id}`);
	}
}
