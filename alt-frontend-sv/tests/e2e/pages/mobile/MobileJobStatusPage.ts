import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Job Status page (/recap/job-status, mobile viewport)
 */
export class MobileJobStatusPage extends BasePage {
	readonly pageTitle: Locator;

	readonly statsRow: Locator;
	readonly successRate: Locator;
	readonly jobsToday: Locator;

	readonly jobCards: Locator;
	readonly detailSheet: Locator;

	readonly activeJobPanel: Locator;
	readonly collapseToggle: Locator;
	readonly pipelineProgress: Locator;
	readonly genreProgressGrid: Locator;

	readonly controlBar: Locator;
	readonly refreshButton: Locator;
	readonly startJobButton: Locator;

	readonly timeWindow24h: Locator;
	readonly timeWindow7d: Locator;

	readonly noJobRunning: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: "Job Status" });

		this.statsRow = page.getByTestId("mobile-stats-row");
		this.successRate = page.getByText("Success rate", { exact: true });
		this.jobsToday = page.getByText("Jobs today", { exact: true });

		this.jobCards = page.getByTestId("mobile-job-card");
		this.detailSheet = page.getByTestId("mobile-job-detail-sheet");

		this.activeJobPanel = page.getByTestId("mobile-active-job-panel");
		this.collapseToggle = page.getByTestId("active-job-collapse-toggle");
		this.pipelineProgress = page.getByTestId("mobile-pipeline-progress");
		this.genreProgressGrid = page.getByTestId("mobile-genre-progress-grid");

		this.controlBar = page.getByTestId("mobile-control-bar");
		this.refreshButton = this.controlBar.getByRole("button", {
			name: /refresh job data/i,
		});
		this.startJobButton = this.controlBar.getByRole("button", {
			name: /start new recap job/i,
		});

		this.timeWindow24h = page.locator('[data-testid="time-window-24h"]');
		this.timeWindow7d = page.locator('[data-testid="time-window-7d"]');

		this.noJobRunning = page.getByText("No active job.");
		this.emptyState = page.getByText("No jobs in this window.");
		this.errorMessage = page.getByText(/error loading/i);
	}

	get url(): string {
		return "./recap/job-status";
	}

	async waitForPageLoaded(): Promise<void> {
		await expect(this.pageTitle).toBeVisible({ timeout: 15000 });
	}

	async openFirstJobDetail(): Promise<void> {
		await this.jobCards.first().click();
		await expect(this.detailSheet).toBeVisible();
	}

	async closeDetailSheet(): Promise<void> {
		const closeBtn = this.detailSheet.getByRole("button", { name: /close/i });
		await closeBtn.click();
		await expect(this.detailSheet).not.toBeVisible();
	}

	async toggleActiveJobPanel(): Promise<void> {
		await this.collapseToggle.click();
	}
}
