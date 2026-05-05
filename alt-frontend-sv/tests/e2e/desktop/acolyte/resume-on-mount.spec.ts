import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import { DesktopAcolyteReportPage } from "../../pages/desktop/DesktopAcolyteReportPage";

const REPORT_ID = "rpt-resume-001";
const RUN_ID = "run-resume-001";

const GET_REPORT_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/GetReport$/;
const LIST_VERSIONS_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/ListReportVersions$/;
const RUN_STATUS_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/GetRunStatus$/;
const LIST_REPORTS_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/ListReports$/;

const REPORT_WITH_ACTIVE_RUN = {
	report: {
		reportId: REPORT_ID,
		title: "Iran Outlook 2026",
		reportType: "weekly_briefing",
		currentVersion: 0,
		createdAt: "2026-05-05T06:05:39Z",
		scope: { topic: "Iran" },
	},
	sections: [],
	activeRun: {
		runId: RUN_ID,
		reportId: REPORT_ID,
		targetVersionNo: 1,
		runStatus: "running",
	},
};

const REPORT_WITHOUT_ACTIVE_RUN = {
	report: REPORT_WITH_ACTIVE_RUN.report,
	sections: [],
};

test.describe("Desktop Acolyte: resume polling on mount via GetReport.active_run", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(LIST_REPORTS_PATH, (route) =>
			fulfillJson(route, { reports: [], nextCursor: "", hasMore: false }),
		);
		await page.route(LIST_VERSIONS_PATH, (route) =>
			fulfillJson(route, { versions: [], nextCursor: "", hasMore: false }),
		);
	});

	test("starts polling automatically when GetReport returns active_run, even without ?run= param", async ({
		page,
	}) => {
		await page.route(GET_REPORT_PATH, (route) =>
			fulfillJson(route, REPORT_WITH_ACTIVE_RUN),
		);

		let statusCalls = 0;
		await page.route(RUN_STATUS_PATH, async (route) => {
			statusCalls += 1;
			await fulfillJson(route, {
				run: {
					runId: RUN_ID,
					reportId: REPORT_ID,
					targetVersionNo: 1,
					runStatus: "running",
				},
			});
		});

		const reportPage = new DesktopAcolyteReportPage(page, REPORT_ID);
		await page.goto(reportPage.url); // No ?run= query — must come from GetReport.active_run

		await expect(reportPage.generatingPill).toBeVisible();
		await expect
			.poll(() => statusCalls, { timeout: 10_000 })
			.toBeGreaterThan(0);
	});

	test("does NOT poll when GetReport.active_run is absent", async ({
		page,
	}) => {
		await page.route(GET_REPORT_PATH, (route) =>
			fulfillJson(route, REPORT_WITHOUT_ACTIVE_RUN),
		);

		let statusCalls = 0;
		await page.route(RUN_STATUS_PATH, async (route) => {
			statusCalls += 1;
			await fulfillJson(route, {
				run: {
					runId: RUN_ID,
					reportId: REPORT_ID,
					targetVersionNo: 1,
					runStatus: "succeeded",
				},
			});
		});

		const reportPage = new DesktopAcolyteReportPage(page, REPORT_ID);
		await page.goto(reportPage.url);
		// Wait long enough that one polling tick (3s) would fire if it were
		// armed. We must observe zero calls.
		await page.waitForTimeout(3500);
		expect(statusCalls).toBe(0);
		await expect(reportPage.generateButton).toBeEnabled();
	});

	test("?run= URL param wins over absent active_run for backward compatibility", async ({
		page,
	}) => {
		// Server says no active run, but URL carries ?run= (from PR1
		// auto-chain navigation). Front-end should still resume polling on
		// the URL-provided runId.
		await page.route(GET_REPORT_PATH, (route) =>
			fulfillJson(route, REPORT_WITHOUT_ACTIVE_RUN),
		);
		let statusCalls = 0;
		await page.route(RUN_STATUS_PATH, async (route) => {
			statusCalls += 1;
			await fulfillJson(route, {
				run: {
					runId: RUN_ID,
					reportId: REPORT_ID,
					targetVersionNo: 1,
					runStatus: "running",
				},
			});
		});

		const reportPage = new DesktopAcolyteReportPage(page, REPORT_ID);
		await page.goto(reportPage.urlWithRun(RUN_ID));
		await expect(reportPage.generatingPill).toBeVisible();
		await expect
			.poll(() => statusCalls, { timeout: 10_000 })
			.toBeGreaterThan(0);
	});
});
