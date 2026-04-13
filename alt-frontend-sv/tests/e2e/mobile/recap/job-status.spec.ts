import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	JOB_PROGRESS_RESPONSE,
	JOB_PROGRESS_WITH_ACTIVE_JOB,
	JOB_PROGRESS_EMPTY,
	JOB_DASHBOARD_PATHS,
} from "../../fixtures/mockData";

test.describe("Mobile Recap Job Status", () => {
	test.use({
		viewport: { width: 375, height: 812 },
	});

	async function setupDefaultMock(page: import("@playwright/test").Page) {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);
	}

	test("renders page header and stats row", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.pageTitle).toBeVisible();
		await expect(mobileJobStatusPage.successRate).toBeVisible();
		await expect(mobileJobStatusPage.jobsToday).toBeVisible();
	});

	test("stats row is horizontally scrollable", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.statsRow).toBeVisible();

		const overflow = await mobileJobStatusPage.statsRow.evaluate(
			(el) => window.getComputedStyle(el).overflowX,
		);
		expect(["auto", "scroll"]).toContain(overflow);
	});

	test("displays job history as card list", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.jobCards.first()).toBeVisible();
		await expect(page.getByText("job-001-")).toBeVisible();
		await expect(
			mobileJobStatusPage.jobCards.first().getByText(/Completed/i),
		).toBeVisible();
	});

	test("tapping job card opens detail sheet", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.jobCards.first()).toBeVisible();
		await mobileJobStatusPage.jobCards.first().click();

		await expect(mobileJobStatusPage.detailSheet).toBeVisible();
		await expect(
			page.getByRole("heading", { name: "Stage duration" }),
		).toBeVisible();
		await expect(
			page.getByRole("heading", { name: "Status history" }),
		).toBeVisible();
	});

	test("displays active job when running", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.successRate).toBeVisible();
		await expect(mobileJobStatusPage.activeJobPanel).toBeVisible();
		await expect(
			page.locator('[data-role="active-job"]').getByText(/Active job/i),
		).toBeVisible();
		await expect(mobileJobStatusPage.pipelineProgress).toBeVisible();
	});

	test("active job panel is collapsible", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.activeJobPanel).toBeVisible();
		await expect(mobileJobStatusPage.collapseToggle).toBeVisible();
		await mobileJobStatusPage.collapseToggle.click();

		await expect(mobileJobStatusPage.pipelineProgress).not.toBeVisible();

		await mobileJobStatusPage.collapseToggle.click();
		await expect(mobileJobStatusPage.pipelineProgress).toBeVisible();
	});

	test("shows no active job message when no active job", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.pageTitle).toBeVisible();
		await expect(mobileJobStatusPage.noJobRunning).toBeVisible();
	});

	test("shows empty state when no jobs", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_EMPTY),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.pageTitle).toBeVisible();
		await expect(mobileJobStatusPage.emptyState).toBeVisible();
	});

	test("time window selector works with horizontal scroll", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.timeWindow24h).toBeVisible();
		await expect(mobileJobStatusPage.timeWindow24h).toHaveAttribute(
			"aria-pressed",
			"true",
		);

		await mobileJobStatusPage.timeWindow7d.click();

		await expect(mobileJobStatusPage.timeWindow7d).toHaveAttribute(
			"aria-pressed",
			"true",
		);
		await expect(mobileJobStatusPage.timeWindow24h).toHaveAttribute(
			"aria-pressed",
			"false",
		);
	});

	test("fixed bottom control bar has refresh and start buttons", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.controlBar).toBeVisible();
		await expect(mobileJobStatusPage.refreshButton).toBeVisible();
		await expect(mobileJobStatusPage.startJobButton).toBeVisible();

		const height = await mobileJobStatusPage.startJobButton.evaluate(
			(el) => el.getBoundingClientRect().height,
		);
		expect(height).toBeGreaterThanOrEqual(44);
	});

	test("refresh button triggers data reload", async ({
		page,
		mobileJobStatusPage,
	}) => {
		let callCount = 0;
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, async (route) => {
			callCount++;
			await fulfillJson(route, JOB_PROGRESS_RESPONSE);
		});

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.successRate).toBeVisible();
		const initialCallCount = callCount;

		await mobileJobStatusPage.refreshButton.click();

		await expect(async () => {
			expect(callCount).toBeGreaterThan(initialCallCount);
		}).toPass({ timeout: 5000 });
	});

	test("start job button triggers job and shows feedback", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);

		await page.route("**/api/v1/generate/recaps/3days", (route) =>
			fulfillJson(route, {
				job_id: "new-job-123",
				genres: ["tech", "ai"],
				status: "running",
			}),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.startJobButton).toBeEnabled();
		await mobileJobStatusPage.startJobButton.click();

		await expect(page.getByText(/Job.*started/i)).toBeVisible({
			timeout: 5000,
		});
	});

	test("start job button disabled when job is running", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.startJobButton).toBeDisabled();
	});

	test("shows error state on API failure", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.errorMessage).toBeVisible({
			timeout: 5000,
		});
	});

	test("pipeline progress shows running stage", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.pipelineProgress).toBeVisible();
		await expect(page.getByText("Fetch")).toBeVisible();
		await expect(page.getByText("Preprocess")).toBeVisible();
		await expect(page.getByText("Evidence")).toBeVisible();

		const currentStage = mobileJobStatusPage.pipelineProgress.locator(
			'[data-stage-status="running"]',
		);
		await expect(currentStage).toBeVisible();
	});

	test("genre progress shows in 2-column grid", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.genreProgressGrid).toBeVisible();
		await expect(page.getByText("tech")).toBeVisible();
	});
});

test.describe("Mobile Job Status - Accessibility", () => {
	test.use({
		viewport: { width: 375, height: 812 },
	});

	test("job cards are keyboard navigable", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.jobCards.first()).toBeVisible();

		await mobileJobStatusPage.jobCards.first().focus();
		await expect(mobileJobStatusPage.jobCards.first()).toBeFocused();

		await page.keyboard.press("Enter");
		await expect(mobileJobStatusPage.detailSheet).toBeVisible();

		await page.keyboard.press("Escape");
		await expect(mobileJobStatusPage.detailSheet).not.toBeVisible();
	});

	test("control bar buttons have proper labels", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./recap/job-status");

		await expect(mobileJobStatusPage.controlBar).toBeVisible();

		await expect(mobileJobStatusPage.refreshButton).toHaveAccessibleName(
			/Refresh job data/i,
		);
		await expect(mobileJobStatusPage.startJobButton).toHaveAccessibleName(
			/Start new recap job/i,
		);
	});
});

test.describe("Mobile Job Status - Touch Interactions", () => {
	test.use({
		viewport: { width: 375, height: 812 },
		hasTouch: true,
	});

	test("bottom sheet can be dismissed by close button", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./recap/job-status");

		await mobileJobStatusPage.jobCards.first().click();
		await expect(mobileJobStatusPage.detailSheet).toBeVisible();

		const closeButton = mobileJobStatusPage.detailSheet.getByRole("button", {
			name: /close/i,
		});
		await closeButton.click();

		await expect(mobileJobStatusPage.detailSheet).not.toBeVisible();
	});
});
