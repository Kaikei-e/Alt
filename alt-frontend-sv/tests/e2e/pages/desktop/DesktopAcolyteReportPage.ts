import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for the desktop Acolyte report detail page
 * (/acolyte/reports/{id}). Auto-polling kicks in when the URL carries
 * `?run=<runId>` so the user does not need to press Generate manually.
 */
export class DesktopAcolyteReportPage extends BasePage {
	private readonly reportId: string;

	readonly title: Locator;
	readonly statusPill: Locator;
	readonly generatingPill: Locator;
	readonly updatedBanner: Locator;
	readonly generateButton: Locator;
	readonly errorBanner: Locator;
	readonly emptyHint: Locator;

	constructor(page: Page, reportId: string) {
		super(page);
		this.reportId = reportId;
		this.title = page.locator(".detail-title");
		this.statusPill = page.locator(".run-status-pill");
		this.generatingPill = page.getByText(/generating/i).first();
		// `.update-banner` is the post-completion refresh banner. Scoping to
		// the class avoids colliding with the `RunStatusPill` (also
		// `role="status"`) when it shows the "Updated" kind.
		this.updatedBanner = page.locator(".update-banner");
		this.generateButton = page.getByRole("button", { name: /^generate$/i });
		this.errorBanner = page.locator(".aco-error");
		this.emptyHint = page.locator(".empty-hint");
	}

	get url(): string {
		return `/acolyte/reports/${this.reportId}`;
	}

	urlWithRun(runId: string): string {
		return `${this.url}?run=${runId}`;
	}

	urlWithAutostartFailed(): string {
		return `${this.url}?autostart_failed=1`;
	}
}
