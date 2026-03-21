import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Evening Pulse page (/desktop/recap/evening-pulse)
 */
export class DesktopEveningPulsePage extends BasePage {
	readonly pageTitle: Locator;
	readonly skeleton: Locator;
	readonly errorState: Locator;
	readonly retryButton: Locator;
	readonly quietDayMessage: Locator;
	readonly pulseView: Locator;
	readonly detailPanel: Locator;
	readonly topicCards: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page
			.getByRole("heading", { name: /evening pulse/i })
			.first();
		this.skeleton = page
			.getByTestId("pulse-skeleton")
			.or(page.locator(".animate-pulse").first());
		this.errorState = page.getByText(/error|failed to load/i).first();
		this.retryButton = page.getByRole("button", { name: /retry/i });
		this.quietDayMessage = page.getByText(/quiet day|no significant/i);
		this.pulseView = page.locator('[aria-label="Evening Pulse topics"]');
		this.detailPanel = page
			.locator('[role="dialog"]')
			.or(page.getByTestId("pulse-detail-panel"));
		this.topicCards = page.locator('button[aria-label^="Topic"]');
	}

	get url(): string {
		return "./recap/evening-pulse";
	}

	/**
	 * Wait for pulse content to load.
	 */
	async waitForPulseLoaded(): Promise<void> {
		await expect(this.pageTitle).toBeVisible({ timeout: 15000 });
	}

	/**
	 * Click on a topic card by index.
	 */
	async selectTopic(index: number): Promise<void> {
		await this.topicCards.nth(index).click();
	}

	/**
	 * Close the detail panel.
	 */
	async closeDetailPanel(): Promise<void> {
		const closeBtn = this.detailPanel
			.getByRole("button", { name: /close/i })
			.first();
		await closeBtn.click();
	}
}
