import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for the desktop "new Acolyte report" page (/acolyte/new).
 * Submitting the form should auto-chain CreateReport → StartReportRun and
 * then navigate to the detail page with the runId in the URL.
 */
export class DesktopAcolyteNewPage extends BasePage {
	readonly heading: Locator;
	readonly titleInput: Locator;
	readonly topicInput: Locator;
	readonly submitButton: Locator;
	readonly errorBanner: Locator;

	constructor(page: Page) {
		super(page);
		this.heading = page.getByRole("heading", { name: /compose new report/i });
		this.titleInput = page.getByLabel(/title/i);
		this.topicInput = page.getByLabel(/topic/i);
		this.submitButton = page.getByRole("button", {
			name: /create.*queue.*generation|create report|generate/i,
		});
		this.errorBanner = page.locator(".aco-error");
	}

	get url(): string {
		return "/acolyte/new";
	}

	async submit(title: string, topic?: string): Promise<void> {
		await this.titleInput.fill(title);
		if (topic) await this.topicInput.fill(topic);
		await this.submitButton.click();
	}
}
