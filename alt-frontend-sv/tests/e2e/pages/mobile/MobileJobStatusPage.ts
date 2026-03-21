import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Job Status page (/mobile/recap/job-status)
 */
export class MobileJobStatusPage extends BasePage {
	// Header
	readonly pageTitle: Locator;

	// Stats
	readonly statsRow: Locator;
	readonly successRate: Locator;
	readonly jobsToday: Locator;

	// Job cards
	readonly jobCards: Locator;
	readonly detailSheet: Locator;

	// Active job
	readonly activeJobPanel: Locator;
	readonly collapseToggle: Locator;
	readonly pipelineProgress: Locator;
	readonly genreProgressGrid: Locator;

	// Control bar
	readonly controlBar: Locator;
	readonly refreshButton: Locator;
	readonly startJobButton: Locator;

	// Time window
	readonly timeWindow24h: Locator;
	readonly timeWindow7d: Locator;

	// States
	readonly noJobRunning: Locator;
	readonly emptyState: Locator;
	readonly errorMessage: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: "Job Status" });

		this.statsRow = page.getByTestId("mobile-stats-row");
		this.successRate = page.getByText("Success Rate");
		this.jobsToday = page.getByText("Jobs Today");

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

		this.timeWindow24h = page.getByRole("button", { name: "24h" });
		this.timeWindow7d = page.getByRole("button", { name: "7d" });

		this.noJobRunning = page.getByText("No job currently running");
		this.emptyState = page.getByText("No jobs found");
		this.errorMessage = page.getByText(/error loading/i);
	}

	get url(): string {
		return "./recap/job-status";
	}

	/**
	 * Wait for the page to load.
	 */
	async waitForPageLoaded(): Promise<void> {
		await expect(this.pageTitle).toBeVisible({ timeout: 15000 });
	}

	/**
	 * Open the detail sheet for the first job.
	 */
	async openFirstJobDetail(): Promise<void> {
		await this.jobCards.first().click();
		await expect(this.detailSheet).toBeVisible();
	}

	/**
	 * Close the detail sheet.
	 */
	async closeDetailSheet(): Promise<void> {
		const closeBtn = this.detailSheet.getByRole("button", { name: /close/i });
		await closeBtn.click();
		await expect(this.detailSheet).not.toBeVisible();
	}

	/**
	 * Toggle the active job panel collapse.
	 */
	async toggleActiveJobPanel(): Promise<void> {
		await this.collapseToggle.click();
	}
}
