import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Evening Pulse page (/mobile/recap/evening-pulse)
 */
export class MobileEveningPulsePage extends BasePage {
	readonly pageTitle: Locator;
	readonly skeleton: Locator;
	readonly errorState: Locator;
	readonly retryButton: Locator;
	readonly quietDayMessage: Locator;
	readonly topicSheet: Locator;
	readonly floatingMenu: Locator;
	readonly topicCards: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page
			.getByRole("heading", { name: /evening pulse/i })
			.first();
		this.skeleton = page.locator(".animate-pulse").first();
		this.errorState = page.getByText(/error|failed to load/i).first();
		this.retryButton = page.getByRole("button", { name: /retry/i });
		this.quietDayMessage = page.getByText(/quiet day|no significant/i);
		this.topicSheet = page
			.getByTestId("pulse-topic-sheet")
			.or(page.locator('[role="dialog"]'));
		this.floatingMenu = page.getByLabel("Open floating menu");
		this.topicCards = page.getByRole("button").filter({
			hasText: /.+/,
		});
	}

	get url(): string {
		return "./recap/evening-pulse";
	}

	/**
	 * Wait for pulse page to load.
	 */
	async waitForPulseLoaded(): Promise<void> {
		await expect(this.pageTitle).toBeVisible({ timeout: 15000 });
	}

	/**
	 * Select a topic card by index.
	 */
	async selectTopic(index: number): Promise<void> {
		await this.topicCards.nth(index).click();
	}
}
