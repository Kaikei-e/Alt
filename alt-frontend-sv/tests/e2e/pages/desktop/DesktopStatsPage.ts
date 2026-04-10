import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Statistics page (/stats)
 * Alt-Paper "Circulation Ledger" editorial layout
 */
export class DesktopStatsPage extends BasePage {
	readonly pageTitle: Locator;

	// Figures bar labels
	readonly feedsLabel: Locator;
	readonly articlesLabel: Locator;
	readonly unsummarizedLabel: Locator;

	// Connection status
	readonly statusLabel: Locator;
	readonly reconnectButton: Locator;

	// Activity log
	readonly activityLogHeading: Locator;
	readonly trendError: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page
			.getByRole("heading", { name: /circulation ledger/i })
			.first();

		this.feedsLabel = page.getByText("FEEDS");
		this.articlesLabel = page.getByText("ARTICLES");
		this.unsummarizedLabel = page.getByText("UNSUMMARIZED");

		this.statusLabel = page.locator(".status-label");
		this.reconnectButton = page.getByRole("button", {
			name: /reconnect/i,
		});

		this.activityLogHeading = page.getByRole("heading", {
			name: /activity log/i,
		});
		this.trendError = page.getByText(/error loading trends/i);
	}

	get url(): string {
		return "./stats";
	}

	/**
	 * Wait for stats to load.
	 */
	async waitForStatsLoaded(): Promise<void> {
		await expect(this.pageTitle).toBeVisible({ timeout: 15000 });
		await expect(this.feedsLabel).toBeVisible({ timeout: 10000 });
	}
}
