import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Recap page
 */
export class MobileRecapPage extends BasePage {
	readonly pageTitle: Locator;
	readonly loadingSpinner: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;

	// Genre tabs (horizontal scroll on mobile)
	readonly genreTabs: Locator;
	readonly recapContent: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: /recap/i }).first();
		this.loadingSpinner = page.locator(".animate-spin").first();
		this.emptyState = page.getByText(/no recap/i);
		this.errorMessage = page.getByText(/error/i);

		this.genreTabs = page.locator("[role='tablist']");
		this.recapContent = page.locator("[role='tabpanel']");
	}

	get url(): string {
		return "./recap";
	}

	async waitForRecapLoaded(): Promise<void> {
		await expect(this.loadingSpinner).not.toBeVisible({ timeout: 15000 });
	}

	getGenreTab(name: string): Locator {
		return this.page.getByRole("tab", { name: new RegExp(name, "i") });
	}

	async selectGenre(name: string): Promise<void> {
		await this.getGenreTab(name).click();
	}

	getEvidenceLinks(): Locator {
		return this.recapContent.locator("a[href]");
	}

	getBulletPoints(): Locator {
		return this.recapContent.locator("li");
	}
}
