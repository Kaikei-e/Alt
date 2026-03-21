import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Statistics page (/stats)
 */
export class DesktopStatsPage extends BasePage {
	readonly pageTitle: Locator;

	// Stat cards
	readonly feedCountCard: Locator;
	readonly totalArticlesCard: Locator;
	readonly unsummarizedCard: Locator;

	// Connection status
	readonly connectionStatus: Locator;
	readonly reconnectButton: Locator;

	// Trend charts
	readonly trendChartsHeading: Locator;
	readonly trendError: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: "Statistics" }).first();

		this.feedCountCard = page.getByText("Feed Count");
		this.totalArticlesCard = page.getByText("Total Articles");
		this.unsummarizedCard = page.getByText("Unsummarized").first();

		this.connectionStatus = page.getByText(/connected|disconnected/i).first();
		this.reconnectButton = page.getByRole("button", {
			name: /reconnect/i,
		});

		this.trendChartsHeading = page.getByRole("heading", {
			name: /trend charts/i,
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
		await expect(this.feedCountCard).toBeVisible({ timeout: 10000 });
	}
}
