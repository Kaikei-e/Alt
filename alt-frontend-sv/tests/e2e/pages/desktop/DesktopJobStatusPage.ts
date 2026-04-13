import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Recap Job Status page (/recap/job-status)
 */
export class DesktopJobStatusPage extends BasePage {
	readonly pageTitle: Locator;
	readonly pageKicker: Locator;

	readonly successRateCard: Locator;
	readonly avgDurationCard: Locator;
	readonly jobsTodayCard: Locator;
	readonly failedJobsCard: Locator;

	readonly recentJobsHeading: Locator;
	readonly jobList: Locator;

	readonly startJobButton: Locator;
	readonly refreshButton: Locator;
	readonly autoRefreshButton: Locator;

	readonly timeWindow24h: Locator;
	readonly timeWindow7d: Locator;

	readonly activeJob: Locator;
	readonly noJobRunning: Locator;

	readonly emptyState: Locator;
	readonly errorMessage: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", { name: "Job Status" });
		this.pageKicker = page.locator('[data-role="page-kicker"]');

		this.successRateCard = page.getByText("Success rate", { exact: true });
		this.avgDurationCard = page.getByText("Avg duration", { exact: true });
		this.jobsTodayCard = page.getByText("Jobs today", { exact: true });
		this.failedJobsCard = page.getByText("Failed jobs", { exact: true });

		this.recentJobsHeading = page.getByRole("heading", { name: "Recent jobs" });
		this.jobList = page.locator('[data-role="recent-jobs"]');

		this.startJobButton = page.locator('[data-role="start-job"]');
		this.refreshButton = page.locator('[data-role="refresh"]');
		this.autoRefreshButton = page.locator('[data-role="auto-refresh"]');

		this.timeWindow24h = page.locator('[data-testid="time-window-24h"]');
		this.timeWindow7d = page.locator('[data-testid="time-window-7d"]');

		this.activeJob = page.locator('[data-role="active-job"]');
		this.noJobRunning = page.getByText("No active job.");

		this.emptyState = page.getByText("No jobs in this window.");
		this.errorMessage = page.getByText(/error loading job data/i);
	}

	get url(): string {
		return "./recap/job-status";
	}

	async waitForPageLoaded(): Promise<void> {
		await expect(this.pageTitle).toBeVisible({ timeout: 15000 });
	}

	getJobRow(jobIdPartial: string): Locator {
		return this.page
			.locator('[data-role="job-row"]')
			.filter({ hasText: jobIdPartial })
			.first();
	}

	async expandJobRow(jobIdPartial: string): Promise<void> {
		const row = this.getJobRow(jobIdPartial);
		await row.locator("button").first().click();
	}

	getExpandableJobRows(): Locator {
		return this.page.locator('[data-role="job-row"] button[aria-expanded]');
	}

	async disableAutoRefresh(): Promise<void> {
		const active = await this.autoRefreshButton.getAttribute("data-active");
		if (active === "true") {
			await this.autoRefreshButton.click();
		}
	}
}
