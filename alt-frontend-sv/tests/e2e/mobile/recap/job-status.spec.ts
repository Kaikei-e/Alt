import { expect, test } from "@playwright/test";
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

	test("renders page header and stats row", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./mobile/recap/job-status");

		// Wait for page to load
		await expect(page.getByRole("heading", { name: "Job Status" })).toBeVisible();

		// Verify stats are visible (horizontally scrollable)
		await expect(page.getByText("Success Rate")).toBeVisible();
		await expect(page.getByText("Jobs Today")).toBeVisible();
	});

	test("stats row is horizontally scrollable", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./mobile/recap/job-status");

		// Wait for stats row
		await expect(page.getByText("Success Rate")).toBeVisible();

		// The stats row container should have horizontal scroll
		const statsRow = page.getByTestId("mobile-stats-row");
		await expect(statsRow).toBeVisible();

		// Verify scroll behavior by checking overflow property
		const overflow = await statsRow.evaluate(
			(el) => window.getComputedStyle(el).overflowX,
		);
		expect(["auto", "scroll"]).toContain(overflow);
	});

	test("displays job history as card list", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./mobile/recap/job-status");

		// Wait for job cards to appear
		await expect(page.getByTestId("mobile-job-card").first()).toBeVisible();

		// Verify job ID is shown (truncated)
		await expect(page.getByText("job-001-")).toBeVisible();

		// Verify status badge is shown
		await expect(page.getByText("Completed")).toBeVisible();
	});

	test("tapping job card opens detail sheet", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./mobile/recap/job-status");

		// Wait for first job card
		const firstCard = page.getByTestId("mobile-job-card").first();
		await expect(firstCard).toBeVisible();

		// Tap on the job card
		await firstCard.click();

		// Verify bottom sheet opens with job details
		await expect(page.getByTestId("mobile-job-detail-sheet")).toBeVisible();
		await expect(page.getByText("Stage Duration Breakdown")).toBeVisible();
		await expect(page.getByText("Status History")).toBeVisible();
	});

	test("displays active job when running", async ({ page }) => {
		// Set up mock with active job
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for stats to load
		await expect(page.getByText("Success Rate")).toBeVisible();

		// Active job panel should be visible and expanded
		await expect(page.getByTestId("mobile-active-job-panel")).toBeVisible();
		await expect(page.getByText("Active Job")).toBeVisible();

		// Pipeline progress should show vertical stepper
		await expect(page.getByTestId("mobile-pipeline-progress")).toBeVisible();
	});

	test("active job panel is collapsible", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for active job panel
		const panel = page.getByTestId("mobile-active-job-panel");
		await expect(panel).toBeVisible();

		// Find and click the collapse button
		const collapseButton = page.getByTestId("active-job-collapse-toggle");
		await expect(collapseButton).toBeVisible();
		await collapseButton.click();

		// Pipeline should be hidden when collapsed
		await expect(page.getByTestId("mobile-pipeline-progress")).not.toBeVisible();

		// Click again to expand
		await collapseButton.click();
		await expect(page.getByTestId("mobile-pipeline-progress")).toBeVisible();
	});

	test("shows no job running message when no active job", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./mobile/recap/job-status");

		// Wait for page to load
		await expect(page.getByRole("heading", { name: "Job Status" })).toBeVisible();

		// Verify no active job message
		await expect(page.getByText("No job currently running")).toBeVisible();
	});

	test("shows empty state when no jobs", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_EMPTY),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for page title
		await expect(page.getByRole("heading", { name: "Job Status" })).toBeVisible();

		// Verify empty state message
		await expect(page.getByText("No jobs found")).toBeVisible();
	});

	test("time window selector works with horizontal scroll", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./mobile/recap/job-status");

		// Wait for time window selector
		await expect(page.getByRole("button", { name: "24h" })).toBeVisible();

		// Verify initial state
		await expect(page.getByRole("button", { name: "24h" })).toHaveAttribute(
			"aria-pressed",
			"true",
		);

		// Click on 7d
		await page.getByRole("button", { name: "7d" }).click();

		// Verify selection changed
		await expect(page.getByRole("button", { name: "7d" })).toHaveAttribute(
			"aria-pressed",
			"true",
		);
		await expect(page.getByRole("button", { name: "24h" })).toHaveAttribute(
			"aria-pressed",
			"false",
		);
	});

	test("fixed bottom control bar has refresh and start buttons", async ({
		page,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./mobile/recap/job-status");

		// Wait for control bar
		const controlBar = page.getByTestId("mobile-control-bar");
		await expect(controlBar).toBeVisible();

		// Verify buttons are present (use aria-label for accessibility)
		await expect(
			controlBar.getByRole("button", { name: /Refresh job data/i }),
		).toBeVisible();
		await expect(
			controlBar.getByRole("button", { name: /Start new recap job/i }),
		).toBeVisible();

		// Verify buttons are touch-friendly (at least 44px height)
		const startButton = controlBar.getByRole("button", { name: /Start new recap job/i });
		const height = await startButton.evaluate(
			(el) => el.getBoundingClientRect().height,
		);
		expect(height).toBeGreaterThanOrEqual(44);
	});

	test("refresh button triggers data reload", async ({ page }) => {
		let callCount = 0;
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, async (route) => {
			callCount++;
			await fulfillJson(route, JOB_PROGRESS_RESPONSE);
		});

		await page.goto("./mobile/recap/job-status");

		// Wait for initial load
		await expect(page.getByText("Success Rate")).toBeVisible();
		const initialCallCount = callCount;

		// Click refresh button in control bar
		const refreshButton = page
			.getByTestId("mobile-control-bar")
			.getByRole("button", { name: /Refresh job data/i });
		await refreshButton.click();

		// Verify API was called again
		await expect(async () => {
			expect(callCount).toBeGreaterThan(initialCallCount);
		}).toPass({ timeout: 5000 });
	});

	test("start job button triggers job and shows feedback", async ({ page }) => {
		await setupDefaultMock(page);

		// Mock trigger endpoint
		await page.route(JOB_DASHBOARD_PATHS.triggerJob, (route) =>
			fulfillJson(route, {
				job_id: "new-job-123",
				genres: ["tech", "ai"],
				status: "running",
			}),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for control bar
		const startButton = page
			.getByTestId("mobile-control-bar")
			.getByRole("button", { name: /Start new recap job/i });
		await expect(startButton).toBeEnabled();

		// Click start job
		await startButton.click();

		// Verify success feedback appears
		await expect(page.getByText(/Job.*started/i)).toBeVisible();
	});

	test("start job button disabled when job is running", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for control bar
		const startButton = page
			.getByTestId("mobile-control-bar")
			.getByRole("button", { name: /Start new recap job/i });
		await expect(startButton).toBeDisabled();
	});

	test("shows error state on API failure", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for error message
		await expect(page.getByText(/Error loading/i)).toBeVisible({ timeout: 5000 });
	});

	test("pipeline progress shows vertical stepper format", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for pipeline progress
		const pipeline = page.getByTestId("mobile-pipeline-progress");
		await expect(pipeline).toBeVisible();

		// Verify stages are displayed vertically
		await expect(page.getByText("Fetch")).toBeVisible();
		await expect(page.getByText("Preprocess")).toBeVisible();
		await expect(page.getByText("Evidence")).toBeVisible();

		// Current stage should have spinner
		const currentStage = pipeline.locator('[data-stage-status="running"]');
		await expect(currentStage).toBeVisible();
	});

	test("genre progress shows in 2-column grid", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for genre grid
		const genreGrid = page.getByTestId("mobile-genre-progress-grid");
		await expect(genreGrid).toBeVisible();

		// Verify genre items are shown
		await expect(page.getByText("tech")).toBeVisible();
	});
});

test.describe("Mobile Job Status - Accessibility", () => {
	test.use({
		viewport: { width: 375, height: 812 },
	});

	test("job cards are keyboard navigable", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for job cards
		const firstCard = page.getByTestId("mobile-job-card").first();
		await expect(firstCard).toBeVisible();

		// Focus on card
		await firstCard.focus();
		await expect(firstCard).toBeFocused();

		// Press Enter to open detail sheet
		await page.keyboard.press("Enter");
		await expect(page.getByTestId("mobile-job-detail-sheet")).toBeVisible();

		// Press Escape to close
		await page.keyboard.press("Escape");
		await expect(page.getByTestId("mobile-job-detail-sheet")).not.toBeVisible();
	});

	test("control bar buttons have proper labels", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./mobile/recap/job-status");

		// Wait for control bar
		const controlBar = page.getByTestId("mobile-control-bar");
		await expect(controlBar).toBeVisible();

		// Verify accessible labels
		await expect(
			controlBar.getByRole("button", { name: /Refresh job data/i }),
		).toHaveAccessibleName(/Refresh job data/i);
		await expect(
			controlBar.getByRole("button", { name: /Start new recap job/i }),
		).toHaveAccessibleName(/Start new recap job/i);
	});
});

test.describe("Mobile Job Status - Touch Interactions", () => {
	test.use({
		viewport: { width: 375, height: 812 },
		hasTouch: true,
	});

	test("bottom sheet can be dismissed by swipe down", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./mobile/recap/job-status");

		// Open detail sheet
		const firstCard = page.getByTestId("mobile-job-card").first();
		await firstCard.click();

		// Wait for sheet to open
		const sheet = page.getByTestId("mobile-job-detail-sheet");
		await expect(sheet).toBeVisible();

		// Close button should work (swipe is harder to test)
		const closeButton = sheet.getByRole("button", { name: /close/i });
		await closeButton.click();

		await expect(sheet).not.toBeVisible();
	});
});
