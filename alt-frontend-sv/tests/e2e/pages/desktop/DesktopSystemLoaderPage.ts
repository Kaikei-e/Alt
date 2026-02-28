import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop System Loader page (/system-loader)
 */
export class DesktopSystemLoaderPage extends BasePage {
	readonly pageTitle: Locator;
	readonly loadingSpinner: Locator;
	readonly progressBar: Locator;
	readonly statusMessage: Locator;
	readonly errorMessage: Locator;
	readonly retryButton: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: /system|loading/i }).first();
		this.loadingSpinner = page.locator(".animate-spin").first();
		this.progressBar = page.locator("[role='progressbar']");
		this.statusMessage = page.getByTestId("status-message");
		this.errorMessage = page.getByText(/error|failed/i);
		this.retryButton = page.getByRole("button", { name: /retry/i });
	}

	get url(): string {
		return "./system-loader";
	}

	async waitForLoadingComplete(timeout = 30000): Promise<void> {
		await expect(this.loadingSpinner).not.toBeVisible({ timeout });
	}

	async getProgress(): Promise<number> {
		const value = await this.progressBar.getAttribute("aria-valuenow");
		return value ? Number.parseInt(value, 10) : 0;
	}

	async retry(): Promise<void> {
		await this.retryButton.click();
	}
}
