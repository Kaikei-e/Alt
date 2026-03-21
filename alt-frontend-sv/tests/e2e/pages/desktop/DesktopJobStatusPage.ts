import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Recap Job Status page (/desktop/recap/job-status)
 */
export class DesktopJobStatusPage extends BasePage {
	// Header
	readonly pageTitle: Locator;

	// Stats cards
	readonly successRateCard: Locator;
	readonly avgDurationCard: Locator;
	readonly jobsTodayCard: Locator;
	readonly failedJobsCard: Locator;

	// Recent jobs section
	readonly recentJobsHeading: Locator;
	readonly jobTable: Locator;

	// Control buttons
	readonly startJobButton: Locator;
	readonly refreshButton: Locator;
	readonly autoRefreshButton: Locator;

	// Time window
	readonly timeWindow24h: Locator;
	readonly timeWindow7d: Locator;

	// Active job
	readonly activeJobHeading: Locator;
	readonly noJobRunning: Locator;

	// States
	readonly emptyState: Locator;
	readonly errorMessage: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", {
			name: "Recap Job Status",
		});

		this.successRateCard = page.getByText("Success Rate");
		this.avgDurationCard = page.getByText("Avg Duration");
		this.jobsTodayCard = page.getByText("Jobs Today");
		this.failedJobsCard = page.getByText("Failed Jobs");

		this.recentJobsHeading = page.getByRole("heading", {
			name: "Recent Jobs",
		});
		this.jobTable = page.locator("table");

		this.startJobButton = page.getByRole("button", { name: "Start Job" });
		this.refreshButton = page.getByRole("button", { name: /^Refresh$/i });
		this.autoRefreshButton = page.getByRole("button", {
			name: /auto-refresh/i,
		});

		this.timeWindow24h = page.locator('[data-testid="time-window-24h"]');
		this.timeWindow7d = page.locator('[data-testid="time-window-7d"]');

		this.activeJobHeading = page.getByRole("heading", {
			name: "Currently Running",
		});
		this.noJobRunning = page.getByText("No job currently running");

		this.emptyState = page.getByText(
			"No jobs found in the selected time window",
		);
		this.errorMessage = page.getByText(/error loading job data/i);
	}

	get url(): string {
		return "./recap/job-status";
	}

	/**
	 * Wait for the job status page to load.
	 */
	async waitForPageLoaded(): Promise<void> {
		await expect(this.pageTitle).toBeVisible({ timeout: 15000 });
	}

	/**
	 * Get a job row by partial job ID.
	 */
	getJobRow(jobIdPartial: string): Locator {
		return this.page.locator("tr").filter({ hasText: jobIdPartial }).first();
	}

	/**
	 * Click on a job row to expand it.
	 */
	async expandJobRow(jobIdPartial: string): Promise<void> {
		await this.getJobRow(jobIdPartial).click();
	}

	/**
	 * Get expandable job rows (with role="button").
	 */
	getExpandableJobRows(): Locator {
		return this.page.locator('tr[role="button"]');
	}

	/**
	 * Turn off auto-refresh if it's on.
	 */
	async disableAutoRefresh(): Promise<void> {
		const text = await this.autoRefreshButton.textContent();
		if (text?.includes("ON")) {
			await this.autoRefreshButton.click();
		}
	}
}
