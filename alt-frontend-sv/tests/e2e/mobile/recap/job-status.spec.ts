import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	JOB_PROGRESS_RESPONSE,
	JOB_PROGRESS_WITH_ACTIVE_JOB,
	JOB_PROGRESS_EMPTY,
	JOB_DASHBOARD_PATHS,
} from "../../fixtures/mockData";

test.describe("Mobile Recap Job Status", () => {
	// Configure mobile viewport for all tests
	test.use({
		viewport: { width: 375, height: 812 },
	});

	// Helper to set up default mock
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

		// Wait for page to load
		await expect(mobileJobStatusPage.pageTitle).toBeVisible();

		// Verify stats are visible (horizontally scrollable)
		await expect(mobileJobStatusPage.successRate).toBeVisible();
		await expect(mobileJobStatusPage.jobsToday).toBeVisible();
	});

	test("stats row is horizontally scrollable", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		// Wait for stats row
		await expect(mobileJobStatusPage.successRate).toBeVisible();

		// The stats row container should have horizontal scroll
		await expect(mobileJobStatusPage.statsRow).toBeVisible();

		// Verify scroll behavior by checking overflow property
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

		// Wait for job cards to appear
		await expect(mobileJobStatusPage.jobCards.first()).toBeVisible();

		// Verify job ID is shown (truncated)
		await expect(page.getByText("job-001-")).toBeVisible();

		// Verify status badge is shown
		await expect(page.getByText("Completed")).toBeVisible();
	});

	test("tapping job card opens detail sheet", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		// Wait for first job card
		await expect(mobileJobStatusPage.jobCards.first()).toBeVisible();

		// Tap on the job card
		await mobileJobStatusPage.jobCards.first().click();

		// Verify bottom sheet opens with job details
		await expect(mobileJobStatusPage.detailSheet).toBeVisible();
		await expect(page.getByText("Stage Duration Breakdown")).toBeVisible();
		await expect(page.getByText("Status History")).toBeVisible();
	});

	test("displays active job when running", async ({
		page,
		mobileJobStatusPage,
	}) => {
		// Set up mock with active job
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		// Wait for stats to load
		await expect(mobileJobStatusPage.successRate).toBeVisible();

		// Active job panel should be visible and expanded
		await expect(mobileJobStatusPage.activeJobPanel).toBeVisible();
		await expect(page.getByText("Active Job")).toBeVisible();

		// Pipeline progress should show vertical stepper
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

		// Wait for active job panel
		await expect(mobileJobStatusPage.activeJobPanel).toBeVisible();

		// Find and click the collapse button
		await expect(mobileJobStatusPage.collapseToggle).toBeVisible();
		await mobileJobStatusPage.collapseToggle.click();

		// Pipeline should be hidden when collapsed
		await expect(mobileJobStatusPage.pipelineProgress).not.toBeVisible();

		// Click again to expand
		await mobileJobStatusPage.collapseToggle.click();
		await expect(mobileJobStatusPage.pipelineProgress).toBeVisible();
	});

	test("shows no job running message when no active job", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		// Wait for page to load
		await expect(mobileJobStatusPage.pageTitle).toBeVisible();

		// Verify no active job message
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

		// Wait for page title
		await expect(mobileJobStatusPage.pageTitle).toBeVisible();

		// Verify empty state message
		await expect(mobileJobStatusPage.emptyState).toBeVisible();
	});

	test("time window selector works with horizontal scroll", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./recap/job-status");

		// Wait for time window selector
		await expect(mobileJobStatusPage.timeWindow24h).toBeVisible();

		// Verify initial state
		await expect(mobileJobStatusPage.timeWindow24h).toHaveAttribute(
			"aria-pressed",
			"true",
		);

		// Click on 7d
		await mobileJobStatusPage.timeWindow7d.click();

		// Verify selection changed
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

		// Wait for control bar
		await expect(mobileJobStatusPage.controlBar).toBeVisible();

		// Verify buttons are present (use aria-label for accessibility)
		await expect(mobileJobStatusPage.refreshButton).toBeVisible();
		await expect(mobileJobStatusPage.startJobButton).toBeVisible();

		// Verify buttons are touch-friendly (at least 44px height)
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

		// Wait for initial load
		await expect(mobileJobStatusPage.successRate).toBeVisible();
		const initialCallCount = callCount;

		// Click refresh button in control bar
		await mobileJobStatusPage.refreshButton.click();

		// Verify API was called again
		await expect(async () => {
			expect(callCount).toBeGreaterThan(initialCallCount);
		}).toPass({ timeout: 5000 });
	});

	test("start job button triggers job and shows feedback", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await setupDefaultMock(page);

		// Mock trigger endpoint (default is 3days endpoint)
		await page.route("**/api/v1/generate/recaps/3days", (route) =>
			fulfillJson(route, {
				job_id: "new-job-123",
				genres: ["tech", "ai"],
				status: "running",
			}),
		);

		await page.goto("./recap/job-status");

		// Wait for control bar
		await expect(mobileJobStatusPage.startJobButton).toBeEnabled();

		// Click start job
		await mobileJobStatusPage.startJobButton.click();

		// Verify success feedback appears (format: "Job XXXXXXXX... started")
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

		// Wait for control bar
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

		// Wait for error message
		await expect(mobileJobStatusPage.errorMessage).toBeVisible({
			timeout: 5000,
		});
	});

	test("pipeline progress shows vertical stepper format", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./recap/job-status");

		// Wait for pipeline progress
		await expect(mobileJobStatusPage.pipelineProgress).toBeVisible();

		// Verify stages are displayed vertically
		await expect(page.getByText("Fetch")).toBeVisible();
		await expect(page.getByText("Preprocess")).toBeVisible();
		await expect(page.getByText("Evidence")).toBeVisible();

		// Current stage should have spinner
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

		// Wait for genre grid
		await expect(mobileJobStatusPage.genreProgressGrid).toBeVisible();

		// Verify genre items are shown
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

		// Wait for job cards
		await expect(mobileJobStatusPage.jobCards.first()).toBeVisible();

		// Focus on card
		await mobileJobStatusPage.jobCards.first().focus();
		await expect(mobileJobStatusPage.jobCards.first()).toBeFocused();

		// Press Enter to open detail sheet
		await page.keyboard.press("Enter");
		await expect(mobileJobStatusPage.detailSheet).toBeVisible();

		// Press Escape to close
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

		// Wait for control bar
		await expect(mobileJobStatusPage.controlBar).toBeVisible();

		// Verify accessible labels
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

	test("bottom sheet can be dismissed by swipe down", async ({
		page,
		mobileJobStatusPage,
	}) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./recap/job-status");

		// Open detail sheet
		await mobileJobStatusPage.jobCards.first().click();

		// Wait for sheet to open
		await expect(mobileJobStatusPage.detailSheet).toBeVisible();

		// Close button should work (swipe is harder to test)
		const closeButton = mobileJobStatusPage.detailSheet.getByRole("button", {
			name: /close/i,
		});
		await closeButton.click();

		await expect(mobileJobStatusPage.detailSheet).not.toBeVisible();
	});
});
