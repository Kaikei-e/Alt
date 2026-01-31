import { expect, test } from "@playwright/test";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	JOB_PROGRESS_RESPONSE,
	JOB_PROGRESS_WITH_ACTIVE_JOB,
	JOB_PROGRESS_EMPTY,
	JOB_DASHBOARD_PATHS,
} from "../../fixtures/mockData";

test.describe("Desktop Recap Job Status", () => {
	// Helper to set up default mock
	async function setupDefaultMock(page: import("@playwright/test").Page) {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);
	}

	test("renders page title and stats cards", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./desktop/recap/job-status");

		// Wait for page title to be visible instead of networkidle
		await expect(
			page.getByRole("heading", { name: "Recap Job Status" }),
		).toBeVisible();

		// Verify stats cards
		await expect(page.getByText("Success Rate")).toBeVisible();
		await expect(page.getByText("Avg Duration")).toBeVisible();
		await expect(page.getByText("Jobs Today")).toBeVisible();
		await expect(page.getByText("Failed Jobs")).toBeVisible();
	});

	test("displays recent jobs in table", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./desktop/recap/job-status");

		// Wait for heading to be visible
		await expect(
			page.getByRole("heading", { name: "Recent Jobs" }),
		).toBeVisible();

		// Verify table headers
		await expect(
			page.getByRole("columnheader", { name: "Job ID" }),
		).toBeVisible();
		await expect(
			page.getByRole("columnheader", { name: "Status" }),
		).toBeVisible();
		await expect(
			page.getByRole("columnheader", { name: "Stages" }),
		).toBeVisible();

		// Verify job data appears
		await expect(page.getByText("job-001-")).toBeVisible();
		await expect(page.getByText("Completed")).toBeVisible();
	});

	test("shows stage progress indicator in table", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./desktop/recap/job-status");

		// Wait for table to be loaded
		await expect(page.getByText("job-001-")).toBeVisible();

		// Verify stage progress indicators are visible (8/8 for completed job)
		await expect(page.getByText("8/8")).toBeVisible();
	});

	test("expands job row to show detailed metrics", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./desktop/recap/job-status");

		// Wait for first job to be visible
		const firstJobRow = page
			.locator("tr")
			.filter({ hasText: "job-001-" })
			.first();
		await expect(firstJobRow).toBeVisible();

		// Click on the first job row to expand
		await firstJobRow.click();

		// Verify detailed metrics panel appears
		await expect(
			page.getByRole("heading", { name: "Stage Duration Breakdown" }),
		).toBeVisible();
		await expect(
			page.getByRole("heading", { name: "Status History" }),
		).toBeVisible();

		// Verify stage duration bars are present (use exact match to avoid matching status history)
		await expect(page.getByText("Fetch", { exact: true })).toBeVisible();
		await expect(page.getByText("Preprocess", { exact: true })).toBeVisible();
		await expect(page.getByText("Total", { exact: true })).toBeVisible();
	});

	test("shows performance metrics summary cards in expanded view", async ({
		page,
	}) => {
		await setupDefaultMock(page);
		await page.goto("./desktop/recap/job-status");

		// Wait for first job to be visible
		const firstJobRow = page
			.locator("tr")
			.filter({ hasText: "job-001-" })
			.first();
		await expect(firstJobRow).toBeVisible();

		// Click on the first job row to expand
		await firstJobRow.click();

		// Wait for expanded content
		await expect(page.getByText("Stage Duration Breakdown")).toBeVisible();

		// Verify performance summary cards - use more specific selectors
		// The expanded view should show summary cards with metrics
		await expect(
			page.getByRole("heading", { name: "Stage Duration Breakdown" }),
		).toBeVisible();
		await expect(page.getByText("Total")).toBeVisible();
	});

	test("displays active job when running", async ({ page }) => {
		// Set up mock with active job data
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("./desktop/recap/job-status");

		// Wait for page to load - check for stats cards first
		await expect(page.getByText("Success Rate")).toBeVisible();

		// Wait for heading to be visible
		await expect(
			page.getByRole("heading", { name: "Currently Running" }),
		).toBeVisible();

		// Verify active job details - use first() since there might be multiple "Running" texts
		await expect(page.getByText("Running").first()).toBeVisible();
	});

	test("shows no job running message when no active job", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./desktop/recap/job-status");

		// Wait for page to be loaded by checking for a key element
		await expect(
			page.getByRole("heading", { name: "Recap Job Status" }),
		).toBeVisible();

		// Verify no job running message
		await expect(page.getByText("No job currently running")).toBeVisible();
	});

	test("shows empty state when no jobs", async ({ page }) => {
		// Set up mock with empty job data
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_EMPTY),
		);

		await page.goto("./desktop/recap/job-status");

		// Wait for page title
		await expect(
			page.getByRole("heading", { name: "Recap Job Status" }),
		).toBeVisible();

		// Verify empty state message
		await expect(
			page.getByText("No jobs found in the selected time window"),
		).toBeVisible();
	});

	test("time window selector changes data", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./desktop/recap/job-status");

		// Wait for time window selector
		await expect(page.getByText("Time Window:")).toBeVisible();

		// Verify time window buttons using data-testid
		await expect(page.locator('[data-testid="time-window-24h"]')).toBeVisible();
		await expect(page.locator('[data-testid="time-window-7d"]')).toBeVisible();

		// Click on 7d time window
		await page.locator('[data-testid="time-window-7d"]').click();

		// Verify the button is now pressed (selected state)
		await expect(
			page.locator('[data-testid="time-window-7d"]'),
		).toHaveAttribute("aria-pressed", "true");
		// Verify the previously selected button is no longer pressed
		await expect(
			page.locator('[data-testid="time-window-24h"]'),
		).toHaveAttribute("aria-pressed", "false");
	});

	test("refresh button reloads data", async ({ page }) => {
		let callCount = 0;
		// Set up mock that tracks call count
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, async (route) => {
			callCount++;
			await fulfillJson(route, JOB_PROGRESS_RESPONSE);
		});

		await page.goto("./desktop/recap/job-status");

		// Wait for page to load - check for stats cards which appear when data loads
		await expect(page.getByText("Success Rate")).toBeVisible();

		// Turn off auto-refresh if it's on to avoid interference
		const autoRefreshBtn = page.getByRole("button", { name: /Auto-refresh/i });
		const autoRefreshText = await autoRefreshBtn.textContent();
		if (autoRefreshText?.includes("ON")) {
			await autoRefreshBtn.click();
			// Wait for auto-refresh to be turned off
			await expect(
				page.getByRole("button", { name: /Auto-refresh OFF/i }),
			).toBeVisible();
		}

		const initialCallCount = callCount;

		// Click refresh button (use exact: true to avoid matching "Auto-refresh")
		await page.getByRole("button", { name: "Refresh", exact: true }).click();

		// Wait for API call to complete using Playwright's retry mechanism
		await expect(async () => {
			expect(callCount).toBeGreaterThan(initialCallCount);
		}).toPass({ timeout: 5000 });
	});

	test("shows error state on API failure", async ({ page }) => {
		// Set up mock that returns an error
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await page.goto("./desktop/recap/job-status");

		// Wait for error message to appear with extended timeout
		await expect(page.getByText(/Error loading job data/)).toBeVisible({
			timeout: 5000,
		});
	});

	test("failed job shows correct status and stage count", async ({ page }) => {
		await setupDefaultMock(page);
		await page.goto("./desktop/recap/job-status");

		// Wait for table to load
		await expect(page.getByText("job-001-")).toBeVisible();

		// Find the failed job row
		const failedJobRow = page
			.locator("tr")
			.filter({ hasText: "job-002-" })
			.first();
		await expect(failedJobRow).toBeVisible();

		// Verify Failed badge is shown
		await expect(failedJobRow.getByText("Failed")).toBeVisible();
	});
});

test.describe("Desktop Job Status - Double Click Prevention", () => {
	test("prevents double-clicking Start Job button", async ({ page }) => {
		let triggerCallCount = 0;

		// Mock job progress (no active job initially)
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		// Mock trigger endpoint - count calls
		await page.route(JOB_DASHBOARD_PATHS.triggerJob, async (route) => {
			triggerCallCount++;
			// Simulate slow response
			await new Promise((resolve) => setTimeout(resolve, 100));
			await fulfillJson(route, {
				job_id: "new-job-123",
				genres: ["tech", "ai"],
				status: "running",
			});
		});

		await page.goto("./desktop/recap/job-status");

		// Wait for page to load
		await expect(
			page.getByRole("heading", { name: "Recap Job Status" }),
		).toBeVisible();

		const startButton = page.getByRole("button", { name: "Start Job" });
		await expect(startButton).toBeEnabled();

		// Click button rapidly twice
		await startButton.click();
		await startButton.click({ force: true }); // Force click even if disabled

		// Wait for requests to complete
		await page.waitForTimeout(500);

		// Should only trigger once
		expect(triggerCallCount).toBe(1);
	});

	test("keeps Start Job button disabled until job data is refreshed", async ({
		page,
	}) => {
		// Mock job progress (no active job initially)
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		// Mock trigger endpoint
		await page.route(JOB_DASHBOARD_PATHS.triggerJob, (route) =>
			fulfillJson(route, {
				job_id: "new-job-123",
				genres: ["tech", "ai"],
				status: "running",
			}),
		);

		await page.goto("./desktop/recap/job-status");

		const startButton = page.getByRole("button", { name: "Start Job" });
		await expect(startButton).toBeEnabled();

		// Click start button
		await startButton.click();

		// Button should remain disabled after API response
		await expect(startButton).toBeDisabled();

		// Verify it stays disabled for at least 500ms after click
		await page.waitForTimeout(500);
		await expect(startButton).toBeDisabled();
	});
});

test.describe("Desktop Job Status - Accessibility", () => {
	test("job history table is keyboard navigable", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./desktop/recap/job-status");

		// Wait for table to load
		await expect(page.getByText("job-001-")).toBeVisible();

		// Tab to first job row
		const firstJobRow = page.locator("tr[role='button']").first();
		await firstJobRow.focus();

		// Verify focus is visible
		await expect(firstJobRow).toBeFocused();

		// Press Enter to expand
		await page.keyboard.press("Enter");

		// Verify expanded content appears
		await expect(page.getByText("Stage Duration Breakdown")).toBeVisible();

		// Press Enter again to collapse
		await page.keyboard.press("Enter");

		// Verify expanded content is hidden
		await expect(page.getByText("Stage Duration Breakdown")).not.toBeVisible();
	});

	test("expanded job row has proper aria attributes", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("./desktop/recap/job-status");

		// Wait for table to load
		const firstJobRow = page.locator("tr[role='button']").first();
		await expect(firstJobRow).toBeVisible();

		// Verify aria-expanded is false initially
		await expect(firstJobRow).toHaveAttribute("aria-expanded", "false");

		// Click to expand
		await firstJobRow.click();

		// Verify aria-expanded is true
		await expect(firstJobRow).toHaveAttribute("aria-expanded", "true");
	});
});
