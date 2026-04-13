import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	JOB_PROGRESS_RESPONSE,
	JOB_PROGRESS_WITH_ACTIVE_JOB,
	JOB_PROGRESS_EMPTY,
	JOB_DASHBOARD_PATHS,
} from "../../fixtures/mockData";

test.describe("Desktop Recap Job Status", () => {
	async function setupDefaultMock(page: import("@playwright/test").Page) {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);
	}

	test("renders page kicker, title, and ledger row", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.pageTitle).toBeVisible();
		await expect(desktopJobStatusPage.pageKicker).toBeVisible();
		await expect(desktopJobStatusPage.pageKicker).toContainText("JOB STATUS");

		await expect(desktopJobStatusPage.successRateCard).toBeVisible();
		await expect(desktopJobStatusPage.avgDurationCard).toBeVisible();
		await expect(desktopJobStatusPage.jobsTodayCard).toBeVisible();
		await expect(desktopJobStatusPage.failedJobsCard).toBeVisible();
	});

	test("displays recent jobs as Alt-Paper rows", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.recentJobsHeading).toBeVisible();
		await expect(page.getByText("job-001-")).toBeVisible();
		await expect(
			page.locator('[data-role="job-row"]').first(),
		).toBeVisible();
	});

	test("status row carries data-status for stripe color", async ({
		page,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(page.getByText("job-001-")).toBeVisible();

		const completedRow = page
			.locator('[data-role="job-row"]')
			.filter({ hasText: "job-001-" })
			.first();
		await expect(completedRow).toHaveAttribute("data-status", "completed");

		const failedRow = page
			.locator('[data-role="job-row"]')
			.filter({ hasText: "job-002-" })
			.first();
		await expect(failedRow).toHaveAttribute("data-status", "failed");
	});

	test("shows stage progress count", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(page.getByText("job-001-")).toBeVisible();
		await expect(page.getByText("8/8").first()).toBeVisible();
	});

	test("expands job row to show detailed metrics", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		const firstJobToggle = page
			.locator('[data-role="job-row"]')
			.filter({ hasText: "job-001-" })
			.first()
			.locator("button");
		await expect(firstJobToggle).toBeVisible();
		await firstJobToggle.click();

		await expect(
			page.getByRole("heading", { name: "Stage duration" }),
		).toBeVisible();
		await expect(
			page.getByRole("heading", { name: "Status history" }),
		).toBeVisible();
		await expect(page.getByText("Fetch", { exact: true })).toBeVisible();
		await expect(page.getByText("Preprocess", { exact: true })).toBeVisible();
		await expect(page.getByText("Total", { exact: true })).toBeVisible();
	});

	test("shows performance summary in expanded view", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		const firstJobToggle = page
			.locator('[data-role="job-row"]')
			.filter({ hasText: "job-001-" })
			.first()
			.locator("button");
		await firstJobToggle.click();

		await expect(
			page.getByRole("heading", { name: "Stage duration" }),
		).toBeVisible();
		await expect(page.getByText("Total", { exact: true })).toBeVisible();
	});

	test("displays active job when running", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.successRateCard).toBeVisible();
		await expect(desktopJobStatusPage.activeJob).toBeVisible();
		await expect(
			desktopJobStatusPage.activeJob.getByText(/Running/i).first(),
		).toBeVisible();
	});

	test("shows no active job message when no active job", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.pageTitle).toBeVisible();
		await expect(desktopJobStatusPage.noJobRunning).toBeVisible();
	});

	test("shows empty state when no jobs", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_EMPTY),
		);

		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.pageTitle).toBeVisible();
		await expect(desktopJobStatusPage.emptyState).toBeVisible();
	});

	test("time window selector changes data", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.timeWindow24h).toBeVisible();
		await expect(desktopJobStatusPage.timeWindow7d).toBeVisible();

		await desktopJobStatusPage.timeWindow7d.click();

		await expect(desktopJobStatusPage.timeWindow7d).toHaveAttribute(
			"aria-pressed",
			"true",
		);
		await expect(desktopJobStatusPage.timeWindow24h).toHaveAttribute(
			"aria-pressed",
			"false",
		);
	});

	test("refresh button reloads data", async ({
		page,
		desktopJobStatusPage,
	}) => {
		let callCount = 0;
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, async (route) => {
			callCount++;
			await fulfillJson(route, JOB_PROGRESS_RESPONSE);
		});

		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.successRateCard).toBeVisible();

		await desktopJobStatusPage.disableAutoRefresh();

		const initialCallCount = callCount;

		await desktopJobStatusPage.refreshButton.click();

		await expect(async () => {
			expect(callCount).toBeGreaterThan(initialCallCount);
		}).toPass({ timeout: 5000 });
	});

	test("shows error state on API failure", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.errorMessage).toBeVisible({
			timeout: 5000,
		});
	});

	test("failed job shows correct status and stage count", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(page.getByText("job-001-")).toBeVisible();

		const failedRow = page
			.locator('[data-role="job-row"]')
			.filter({ hasText: "job-002-" })
			.first();
		await expect(failedRow).toBeVisible();
		await expect(failedRow.getByText(/Failed/i)).toBeVisible();
	});
});

test.describe("Desktop Job Status - Job Trigger", () => {
	test("Start Job button is disabled when a job is already running", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.pageTitle).toBeVisible();
		await expect(desktopJobStatusPage.startJobButton).toBeDisabled();
	});

	test("Start Job button is enabled when no job is running", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.pageTitle).toBeVisible();
		await expect(desktopJobStatusPage.startJobButton).toBeEnabled();
	});

	test("shows success feedback after starting job", async ({
		page,
		desktopJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.route("**/api/v1/generate/recaps/3days", (route) =>
			fulfillJson(route, {
				job_id: "new-job-123",
				genres: ["tech", "ai"],
				status: "running",
			}),
		);

		await page.goto("./recap/job-status");

		await expect(desktopJobStatusPage.startJobButton).toBeEnabled();
		await desktopJobStatusPage.startJobButton.click();

		await expect(page.getByText(/Job.*started/i)).toBeVisible({
			timeout: 5000,
		});
	});
});

test.describe("Desktop Job Status - Accessibility", () => {
	test("job row toggle is keyboard navigable", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./recap/job-status");

		await expect(page.getByText("job-001-")).toBeVisible();

		const firstToggle = page
			.locator('[data-role="job-row"] button[aria-expanded]')
			.first();
		await firstToggle.focus();
		await expect(firstToggle).toBeFocused();

		await page.keyboard.press("Enter");
		await expect(
			page.getByRole("heading", { name: "Stage duration" }),
		).toBeVisible();

		await page.keyboard.press("Enter");
		await expect(
			page.getByRole("heading", { name: "Stage duration" }),
		).not.toBeVisible();
	});

	test("expanded job row has proper aria attributes", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./recap/job-status");

		const firstToggle = page
			.locator('[data-role="job-row"] button[aria-expanded]')
			.first();
		await expect(firstToggle).toBeVisible();

		await expect(firstToggle).toHaveAttribute("aria-expanded", "false");

		await firstToggle.click();
		await expect(firstToggle).toHaveAttribute("aria-expanded", "true");
	});
});
