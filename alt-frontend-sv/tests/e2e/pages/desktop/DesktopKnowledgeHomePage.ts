import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Knowledge Home (desktop view).
 * Exposes locators for the main feed, TodayBar, RecallRail, and card interactions.
 */
export class DesktopKnowledgeHomePage extends BasePage {
	// TodayBar
	readonly todayBar: Locator;

	// Knowledge Stream
	readonly knowledgeStream: Locator;

	// Cards
	readonly cards: Locator;

	// Recall Rail
	readonly recallRail: Locator;
	readonly recallCandidateCards: Locator;

	// Stream Update Bar
	readonly streamUpdateBar: Locator;

	// Degraded banner
	readonly degradedBanner: Locator;

	constructor(page: Page) {
		super(page);
		this.todayBar = page
			.locator("[data-testid='today-bar']")
			.or(page.getByText("Knowledge Home").first());
		this.knowledgeStream = page.locator("[data-item-key]").first();
		this.cards = page.locator("article[data-item-key]");
		this.recallRail = page.getByText("Recall").first();
		this.recallCandidateCards = page.locator("[role='button'][tabindex='0']");
		this.streamUpdateBar = page.getByText("items updated");
		this.degradedBanner = page.getByText("degraded");
	}

	get url(): string {
		return "/home";
	}

	/** Get a specific card by item key */
	getCard(itemKey: string): Locator {
		return this.page.locator(`article[data-item-key="${itemKey}"]`);
	}

	/** Get the summary excerpt text within a card */
	getCardSummary(itemKey: string): Locator {
		return this.getCard(itemKey).locator("p.line-clamp-2").first();
	}

	/** Get the "Summarizing" chip within a card */
	getSummarizingChip(itemKey: string): Locator {
		return this.getCard(itemKey).getByText("Summarizing");
	}

	/** Get skeleton placeholders within a card */
	getCardSkeleton(itemKey: string): Locator {
		return this.getCard(itemKey).locator(".animate-pulse");
	}

	/** Get tag chips within a card */
	getCardTags(itemKey: string): Locator {
		return this.getCard(itemKey).locator('a[href^="/articles/by-tag"]');
	}

	/** Get the Open button within a card's QuickActionRow */
	getOpenButton(itemKey: string): Locator {
		return this.getCard(itemKey).getByLabel("Open");
	}

	/** Get the "Why?" button in a recall candidate card */
	getRecallWhyButton(): Locator {
		return this.page.getByText("Why?").first();
	}

	/** Get the "Why recalled?" panel */
	getRecallWhyPanel(): Locator {
		return this.page.getByText("Why recalled?").first();
	}

	/** Get recall reason badges */
	getRecallReasonBadges(): Locator {
		return this.page.locator("[class*='badge']");
	}

	/** Get the supersede badge on a card */
	getSupersedeBadge(itemKey: string): Locator {
		return this.getCard(itemKey).getByText(/updated|replaced/i);
	}

	/** Get the supersede detail panel */
	getSupersedDetail(): Locator {
		return this.page.getByText("Previous summary:").first();
	}

	/** Get the stream update refresh button */
	getStreamRefreshButton(): Locator {
		return this.page.locator("button").filter({ hasText: /updated/ });
	}

	/** Get lens tab buttons */
	getLensTab(name: string): Locator {
		return this.page.locator("button").filter({ hasText: new RegExp(`^${name}$`) });
	}

	/** Get the morning letter link in TodayBar */
	getTodayBarMorningLetterLink(): Locator {
		return this.todayBar.locator('a[href*="morning-letter"]');
	}

	/** Get the evening pulse link in TodayBar */
	getTodayBarEveningPulseLink(): Locator {
		return this.todayBar.locator('a[href*="evening-pulse"]');
	}

	/** Get the weekly recap link in TodayBar */
	getTodayBarRecapLink(): Locator {
		return this.todayBar.locator('a[href*="/recap"]').first();
	}

	/** Wait for the home page to finish initial loading */
	async waitForHomeLoaded(): Promise<void> {
		await this.page.waitForLoadState("domcontentloaded");
		// Wait for at least one card or the empty state
		await this.cards
			.first()
			.or(this.page.getByText("No articles yet"))
			.waitFor({ timeout: 15000 });
	}
}
