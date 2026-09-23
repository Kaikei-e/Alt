import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Topic Cards page (/recap/cards)
 */
export class DesktopRecapCardsPage extends BasePage {
	readonly pageTitle: Locator;
	readonly jobWindow: Locator;
	readonly loadingSkeleton: Locator;
	readonly emptyState: Locator;
	readonly degradedNotice: Locator;
	readonly errorMessage: Locator;
	readonly retryButton: Locator;
	readonly cards: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: /topic cards/i });
		this.jobWindow = page.getByTestId("recap-cards-window");
		this.loadingSkeleton = page.getByTestId("recap-cards-skeleton");
		this.emptyState = page.getByTestId("recap-cards-empty");
		this.degradedNotice = page.getByTestId("recap-cards-degraded");
		this.errorMessage = page.getByTestId("recap-cards-error");
		this.retryButton = page.getByRole("button", { name: /retry/i });
		this.cards = page.getByTestId("recap-topic-card");
	}

	get url(): string {
		return "./recap/cards";
	}

	async waitForLoaded(): Promise<void> {
		await expect(this.loadingSkeleton).not.toBeVisible({ timeout: 15000 });
	}

	getCardByRank(rank: number): Locator {
		return this.page.locator(
			`[data-testid="recap-topic-card"][data-rank="${rank}"]`,
		);
	}

	getCardHeadline(rank: number): Locator {
		return this.getCardByRank(rank).getByTestId("card-headline");
	}

	getCardWhat(rank: number): Locator {
		return this.getCardByRank(rank).getByTestId("card-what");
	}

	getCardWhy(rank: number): Locator {
		return this.getCardByRank(rank).getByTestId("card-why");
	}

	getCardGenre(rank: number): Locator {
		return this.getCardByRank(rank).getByTestId("card-genre");
	}

	getCardContinues(rank: number): Locator {
		return this.getCardByRank(rank).getByTestId("card-continues");
	}

	getCardSources(rank: number): Locator {
		return this.getCardByRank(rank).getByTestId("card-source");
	}
}
