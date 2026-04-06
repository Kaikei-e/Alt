import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Knowledge Home (mobile view).
 */
export class MobileKnowledgeHomePage extends BasePage {
	readonly cards: Locator;
	readonly recallSection: Locator;

	constructor(page: Page) {
		super(page);
		this.cards = page.locator("article[data-item-key]");
		this.recallSection = page.getByText("Recall").first();
	}

	get url(): string {
		return "/home";
	}

	getCard(itemKey: string): Locator {
		return this.page.locator(`article[data-item-key="${itemKey}"]`);
	}

	getCardSummary(itemKey: string): Locator {
		return this.getCard(itemKey).locator("p.line-clamp-2").first();
	}

	getSummarizingChip(itemKey: string): Locator {
		return this.getCard(itemKey).getByText("Summarizing");
	}

	async waitForHomeLoaded(): Promise<void> {
		await this.page.waitForLoadState("domcontentloaded");
		await this.cards
			.first()
			.or(this.page.getByText("No articles yet"))
			.waitFor({ timeout: 15000 });
	}
}
