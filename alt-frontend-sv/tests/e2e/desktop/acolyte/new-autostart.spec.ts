import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import { DesktopAcolyteNewPage } from "../../pages/desktop/DesktopAcolyteNewPage";
import { DesktopAcolyteReportPage } from "../../pages/desktop/DesktopAcolyteReportPage";

const REPORT_ID = "rpt-autostart-001";
const RUN_ID = "run-autostart-001";

const CREATE_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/CreateReport$/;
const START_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/StartReportRun$/;
const GET_REPORT_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/GetReport$/;
const LIST_VERSIONS_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/ListReportVersions$/;
const RUN_STATUS_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/GetRunStatus$/;
const LIST_REPORTS_PATH =
	/\/api\/v2\/alt\.acolyte\.v1\.AcolyteService\/ListReports$/;

const REPORT_PAYLOAD = {
	report: {
		reportId: REPORT_ID,
		title: "Iran Outlook 2026",
		reportType: "weekly_briefing",
		currentVersion: 0,
		createdAt: "2026-05-05T06:05:39Z",
		scope: { topic: "Iran" },
	},
	sections: [],
};

test.describe("Desktop Acolyte: auto-chain create → start run", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(LIST_REPORTS_PATH, (route) =>
			fulfillJson(route, { reports: [], nextCursor: "", hasMore: false }),
		);
		await page.route(GET_REPORT_PATH, (route) =>
			fulfillJson(route, REPORT_PAYLOAD),
		);
		await page.route(LIST_VERSIONS_PATH, (route) =>
			fulfillJson(route, { versions: [], nextCursor: "", hasMore: false }),
		);
	});

	test("submits CreateReport then StartReportRun and lands on detail with ?run=", async ({
		page,
	}) => {
		const calls: string[] = [];
		await page.route(CREATE_PATH, async (route) => {
			calls.push("CreateReport");
			await fulfillJson(route, { reportId: REPORT_ID });
		});
		await page.route(START_PATH, async (route) => {
			calls.push("StartReportRun");
			await fulfillJson(route, { runId: RUN_ID });
		});
		await page.route(RUN_STATUS_PATH, (route) =>
			fulfillJson(route, {
				run: {
					runId: RUN_ID,
					reportId: REPORT_ID,
					targetVersionNo: 1,
					runStatus: "running",
				},
			}),
		);

		const newPage = new DesktopAcolyteNewPage(page);
		const reportPage = new DesktopAcolyteReportPage(page, REPORT_ID);
		await newPage.goto();
		await newPage.submit("Iran Outlook 2026");

		await expect(page).toHaveURL(
			new RegExp(`/acolyte/reports/${REPORT_ID}\\?run=${RUN_ID}`),
		);
		expect(calls).toEqual(["CreateReport", "StartReportRun"]);
		// Detail page picks up ?run= and shows the Generating pill without
		// the user having to press the Generate button.
		await expect(reportPage.generatingPill).toBeVisible();
	});

	test("polling reaches succeeded and the URL strips ?run=", async ({
		page,
	}) => {
		await page.route(CREATE_PATH, (route) =>
			fulfillJson(route, { reportId: REPORT_ID }),
		);
		await page.route(START_PATH, (route) =>
			fulfillJson(route, { runId: RUN_ID }),
		);

		let statusCalls = 0;
		await page.route(RUN_STATUS_PATH, async (route) => {
			statusCalls += 1;
			await fulfillJson(route, {
				run: {
					runId: RUN_ID,
					reportId: REPORT_ID,
					targetVersionNo: 1,
					runStatus: statusCalls === 1 ? "running" : "succeeded",
				},
			});
		});

		const newPage = new DesktopAcolyteNewPage(page);
		const reportPage = new DesktopAcolyteReportPage(page, REPORT_ID);
		await newPage.goto();
		await newPage.submit("Iran Outlook 2026");

		await expect(page).toHaveURL(
			new RegExp(`/acolyte/reports/${REPORT_ID}\\?run=${RUN_ID}`),
		);
		// First tick: running → still has ?run=. Wait through one poll and
		// observe the URL strip after the terminal succeeded tick.
		await expect(page).toHaveURL(new RegExp(`/acolyte/reports/${REPORT_ID}$`), {
			timeout: 15_000,
		});
		await expect(reportPage.updatedBanner).toBeVisible();
	});

	test("StartReportRun failure redirects with autostart_failed=1 and shows error + manual Generate", async ({
		page,
	}) => {
		await page.route(CREATE_PATH, (route) =>
			fulfillJson(route, { reportId: REPORT_ID }),
		);
		await page.route(START_PATH, (route) =>
			fulfillJson(
				route,
				{ code: "failed_precondition", message: "already running" },
				412,
			),
		);

		const newPage = new DesktopAcolyteNewPage(page);
		const reportPage = new DesktopAcolyteReportPage(page, REPORT_ID);
		await newPage.goto();
		await newPage.submit("Iran Outlook 2026");

		await expect(page).toHaveURL(
			new RegExp(`/acolyte/reports/${REPORT_ID}\\?autostart_failed=1`),
		);
		await expect(reportPage.errorBanner).toBeVisible();
		await expect(reportPage.generateButton).toBeEnabled();
	});

	test("CreateReport failure does NOT navigate and shows inline error", async ({
		page,
	}) => {
		const navigations: string[] = [];
		page.on("framenavigated", (frame) => {
			if (frame === page.mainFrame()) navigations.push(frame.url());
		});

		await page.route(CREATE_PATH, (route) =>
			fulfillJson(route, { code: "internal", message: "boom" }, 500),
		);
		// Should not be hit — but stub it so the test fails loudly if the
		// chain accidentally calls StartReportRun after a CreateReport error.
		await page.route(START_PATH, (route) =>
			fulfillJson(route, { runId: RUN_ID }),
		);

		const newPage = new DesktopAcolyteNewPage(page);
		await newPage.goto();
		await newPage.submit("Iran Outlook 2026");

		await expect(newPage.errorBanner).toBeVisible();
		// Stayed on /acolyte/new.
		await expect(page).toHaveURL(/\/acolyte\/new$/);
		// No navigation toward /reports/.
		expect(navigations.some((u) => /\/acolyte\/reports\//.test(u))).toBe(false);
	});
});
