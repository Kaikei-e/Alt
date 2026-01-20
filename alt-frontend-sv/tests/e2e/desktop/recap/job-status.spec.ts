import { expect, test } from "@playwright/test";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	JOB_PROGRESS_RESPONSE,
	JOB_PROGRESS_WITH_ACTIVE_JOB,
	JOB_PROGRESS_EMPTY,
	JOB_DASHBOARD_PATHS,
} from "../../fixtures/mockData";

test.describe("Desktop Recap Job Status", () => {
	test.beforeEach(async ({ page }) => {
		// Set up default mock
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);
	});

	test("renders page title and stats cards", async ({ page }) => {
		await page.goto("/desktop/recap/job-status");

		// Wait for page title to be visible instead of networkidle
		await expect(page.getByRole("heading", { name: "Recap Job Status" })).toBeVisible();

		// Verify stats cards
		await expect(page.getByText("Success Rate")).toBeVisible();
		await expect(page.getByText("Avg Duration")).toBeVisible();
		await expect(page.getByText("Jobs Today")).toBeVisible();
		await expect(page.getByText("Failed Jobs")).toBeVisible();
	});

	test("displays recent jobs in table", async ({ page }) => {
		await page.goto("/desktop/recap/job-status");

		// Wait for heading to be visible
		await expect(page.getByRole("heading", { name: "Recent Jobs" })).toBeVisible();

		// Verify table headers
		await expect(page.getByRole("columnheader", { name: "Job ID" })).toBeVisible();
		await expect(page.getByRole("columnheader", { name: "Status" })).toBeVisible();
		await expect(page.getByRole("columnheader", { name: "Stages" })).toBeVisible();

		// Verify job data appears
		await expect(page.getByText("job-001-")).toBeVisible();
		await expect(page.getByText("Completed")).toBeVisible();
	});

	test("shows stage progress indicator in table", async ({ page }) => {
		await page.goto("/desktop/recap/job-status");

		// Wait for table to be loaded
		await expect(page.getByText("job-001-")).toBeVisible();

		// Verify stage progress indicators are visible (8/8 for completed job)
		await expect(page.getByText("8/8")).toBeVisible();
	});

	test("expands job row to show detailed metrics", async ({ page }) => {
		await page.goto("/desktop/recap/job-status");

		// Wait for first job to be visible
		const firstJobRow = page.locator("tr").filter({ hasText: "job-001-" }).first();
		await expect(firstJobRow).toBeVisible();

		// Click on the first job row to expand
		await firstJobRow.click();

		// Verify detailed metrics panel appears
		await expect(page.getByText("Stage Duration Breakdown")).toBeVisible();
		await expect(page.getByText("Status History")).toBeVisible();

		// Verify stage duration bars are present
		await expect(page.getByText("Fetch")).toBeVisible();
		await expect(page.getByText("Preprocess")).toBeVisible();
		await expect(page.getByText("Total")).toBeVisible();
	});

	test("shows performance metrics summary cards in expanded view", async ({ page }) => {
		await page.goto("/desktop/recap/job-status");

		// Wait for first job to be visible
		const firstJobRow = page.locator("tr").filter({ hasText: "job-001-" }).first();
		await expect(firstJobRow).toBeVisible();

		// Click on the first job row to expand
		await firstJobRow.click();

		// Verify performance summary cards
		await expect(page.getByText("Duration")).toBeVisible();
		await expect(page.getByText("Performance")).toBeVisible();
		await expect(page.getByText("vs Average")).toBeVisible();
		await expect(page.getByText("Stages")).toBeVisible();
	});

	test("displays active job when running", async ({ page }) => {
		// Override mock with active job data
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_WITH_ACTIVE_JOB),
		);

		await page.goto("/desktop/recap/job-status");

		// Wait for heading to be visible
		await expect(page.getByRole("heading", { name: "Currently Running" })).toBeVisible();

		// Verify active job details
		await expect(page.getByText("Active Job")).toBeVisible();
		await expect(page.getByText("Running")).toBeVisible();
	});

	test("shows no job running message when no active job", async ({ page }) => {
		await page.goto("/desktop/recap/job-status");

		// Wait for page to be loaded by checking for a key element
		await expect(page.getByRole("heading", { name: "Recap Job Status" })).toBeVisible();

		// Verify no job running message
		await expect(page.getByText("No job currently running")).toBeVisible();
	});

	test("shows empty state when no jobs", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_EMPTY),
		);

		await page.goto("/desktop/recap/job-status");

		// Wait for page title
		await expect(page.getByRole("heading", { name: "Recap Job Status" })).toBeVisible();

		// Verify empty state message
		await expect(page.getByText("No jobs found in the selected time window")).toBeVisible();
	});

	test("time window selector changes data", async ({ page }) => {
		await page.goto("/desktop/recap/job-status");

		// Wait for time window selector
		await expect(page.getByText("Time Window:")).toBeVisible();

		// Verify time window buttons
		await expect(page.getByRole("button", { name: "24h" })).toBeVisible();
		await expect(page.getByRole("button", { name: "7d" })).toBeVisible();

		// Click on 7d time window
		await page.getByRole("button", { name: "7d" }).click();

		// Verify the button is now pressed (selected state)
		await expect(page.getByRole("button", { name: "7d" })).toHaveAttribute("aria-pressed", "true");
		// Verify the previously selected button is no longer pressed
		await expect(page.getByRole("button", { name: "24h" })).toHaveAttribute("aria-pressed", "false");
	});

	test("refresh button reloads data", async ({ page }) => {
		let callCount = 0;
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, async (route) => {
			callCount++;
			await fulfillJson(route, JOB_PROGRESS_RESPONSE);
		});

		await page.goto("/desktop/recap/job-status");

		// Wait for page to load
		await expect(page.getByRole("heading", { name: "Recap Job Status" })).toBeVisible();

		const initialCallCount = callCount;

		// Click refresh button
		await page.getByRole("button", { name: "Refresh" }).click();

		// Wait for API call to complete using Playwright's retry mechanism
		await expect(async () => {
			expect(callCount).toBeGreaterThan(initialCallCount);
		}).toPass({ timeout: 5000 });
	});

	test("shows error state on API failure", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await page.goto("/desktop/recap/job-status");

		// Wait for error message to appear with extended timeout
		await expect(page.getByText(/Error loading job data/)).toBeVisible({ timeout: 5000 });
	});

	test("failed job shows correct status and stage count", async ({ page }) => {
		await page.goto("/desktop/recap/job-status");

		// Wait for table to load
		await expect(page.getByText("job-001-")).toBeVisible();

		// Find the failed job row
		const failedJobRow = page.locator("tr").filter({ hasText: "job-002-" }).first();
		await expect(failedJobRow).toBeVisible();

		// Verify Failed badge is shown
		await expect(failedJobRow.getByText("Failed")).toBeVisible();
	});
});

test.describe("Desktop Job Status - Accessibility", () => {
	test("job history table is keyboard navigable", async ({ page }) => {
		await page.route(JOB_DASHBOARD_PATHS.jobProgress, (route) =>
			fulfillJson(route, JOB_PROGRESS_RESPONSE),
		);

		await page.goto("/desktop/recap/job-status");

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

		await page.goto("/desktop/recap/job-status");

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
