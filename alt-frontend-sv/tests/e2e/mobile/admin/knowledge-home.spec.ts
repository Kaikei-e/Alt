import type { Page } from "@playwright/test";
import { fulfillWatchStream } from "../../desktop/admin/admin-monitor.fixture";
import {
	AUDIT_RESULT,
	RETENTION_RUN_RESULT,
	SOVEREIGN_ADMIN_SNAPSHOT,
} from "../../fixtures/factories/sovereignAdminFactory";
import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";

const ADMIN_API = "**/api/admin/knowledge-home";
const SOVEREIGN_API = "**/api/admin/knowledge-home/sovereign";
const WATCH_URL = "**/api/v2/alt.admin_monitor.v1.AdminMonitorService/Watch";

const RICH_ADMIN_SNAPSHOT = {
	health: {
		activeVersion: 2,
		checkpointSeq: 1_234_567,
		lastUpdated: "2026-11-27T12:34:56Z",
		backfillJobs: [
			{
				jobId: "job-running-001",
				status: "running",
				projectionVersion: 2,
				totalEvents: 150_000,
				processedEvents: 75_000,
				errorMessage:
					"transient timeout talking to knowledge-sovereign upstream endpoint",
				createdAt: "2026-11-27T11:00:00Z",
				startedAt: "2026-11-27T11:00:05Z",
				completedAt: "",
			},
			{
				jobId: "job-paused-002",
				status: "paused",
				projectionVersion: 1,
				totalEvents: 50_000,
				processedEvents: 12_500,
				errorMessage: "",
				createdAt: "2026-11-26T09:15:00Z",
				startedAt: "2026-11-26T09:15:10Z",
				completedAt: "",
			},
		],
	},
	flags: {
		enableHomePage: true,
		enableTracking: true,
		enableProjectionV2: true,
		rolloutPercentage: 100,
		enableRecallRail: true,
		enableLens: true,
		enableStreamUpdates: true,
		enableSupersedeUx: false,
	},
	sloStatus: {
		overallHealth: "at_risk",
		slis: [
			{
				name: "availability",
				currentValue: 99.82,
				targetValue: 99.9,
				unit: "%",
				status: "burning",
				errorBudgetConsumedPct: 68.4,
			},
			{
				name: "freshness",
				currentValue: 18.5,
				targetValue: 60.0,
				unit: "s",
				status: "meeting",
				errorBudgetConsumedPct: 12.1,
			},
			{
				name: "action_durability",
				currentValue: 99.99,
				targetValue: 99.95,
				unit: "%",
				status: "meeting",
				errorBudgetConsumedPct: 4.0,
			},
			{
				name: "stream_continuity",
				currentValue: 96.4,
				targetValue: 99.0,
				unit: "%",
				status: "breached",
				errorBudgetConsumedPct: 92.5,
			},
		],
		errorBudgetWindowDays: 30,
		computedAt: "2026-11-27T12:34:56Z",
		activeAlerts: [
			{
				alertName: "KnowledgeHomeStreamDisconnectSpike",
				severity: "critical",
				status: "firing",
				firedAt: "2026-11-27T12:20:00Z",
				description:
					"SSE stream disconnect rate exceeded 20% over the last 10m window on knowledge-home.",
			},
		],
	},
	reprojectRuns: [
		{
			reprojectRunId: "reproj-run-001",
			projectionName: "knowledge_home",
			fromVersion: "v1",
			toVersion: "v2",
			mode: "full",
			status: "swappable",
			createdAt: "2026-11-27T08:00:00Z",
			startedAt: "2026-11-27T08:00:10Z",
			finishedAt: "2026-11-27T08:42:15Z",
		},
		{
			reprojectRunId: "reproj-run-002",
			projectionName: "knowledge_home",
			fromVersion: "v0",
			toVersion: "v1",
			mode: "time_range",
			status: "swapped",
			createdAt: "2026-11-20T06:00:00Z",
			startedAt: "2026-11-20T06:00:05Z",
			finishedAt: "2026-11-20T06:18:30Z",
		},
	],
	systemMetrics: {
		projector: {
			eventsProcessed: 786_652,
			lagSeconds: 42,
			batchDurationMsP50: 18,
			batchDurationMsP95: 94,
			batchDurationMsP99: 210,
			errors: 0,
		},
		handler: {
			pagesServed: 12_400,
			pagesDegraded: 3,
			degradedRatePct: 0.02,
		},
		tracking: {
			itemsExposed: 48_200,
			itemsOpened: 9_640,
			itemsDismissed: 1_210,
			openRatePct: 20.0,
			dismissRatePct: 2.5,
		},
		stream: {
			connectionsTotal: 820,
			disconnectsTotal: 41,
			reconnectsTotal: 39,
			deliveriesTotal: 15_230,
			disconnectRatePct: 5.0,
		},
		correctness: {
			emptySummaryCount: 12,
			malformedWhyCount: 0,
			orphanItemsCount: 0,
			supersedeMismatchCount: 0,
			requestsTotal: 12_400,
			correctnessScorePct: 99.9,
		},
		sovereign: {
			mutationsApplied: 92_410,
			mutationsErrors: 1,
			mutationDurationMsP50: 6,
			mutationDurationMsP95: 28,
			errorRatePct: 0.001,
		},
		recall: {
			signalsAppended: 31_400,
			signalErrors: 0,
			candidatesGenerated: 4_820,
			candidatesEmpty: 19,
			usersProcessed: 42,
			projectorDurationMsP50: 14,
			projectorDurationMsP95: 62,
		},
		serviceHealth: [
			{
				serviceName: "knowledge-sovereign",
				endpoint: "http://knowledge-sovereign:9501/health",
				status: "healthy",
				latencyMs: 8,
				checkedAt: "2026-11-27T12:34:50Z",
				errorMessage: "",
			},
			{
				serviceName: "alt-butterfly-facade",
				endpoint: "http://alt-butterfly-facade:9250/health",
				status: "healthy",
				latencyMs: 12,
				checkedAt: "2026-11-27T12:34:50Z",
				errorMessage: "",
			},
			{
				serviceName: "rask-log-aggregator",
				endpoint: "http://rask-log-aggregator:9600/health",
				status: "unhealthy",
				latencyMs: 5002,
				checkedAt: "2026-11-27T12:34:50Z",
				errorMessage:
					"context deadline exceeded while connecting to upstream collector endpoint",
			},
		],
	},
};

const REPROJECT_DIFF_RESPONSE = {
	ok: true,
	diff: {
		fromItemCount: 14_820,
		toItemCount: 15_012,
		fromEmptyCount: 412,
		toEmptyCount: 12,
		fromAvgScore: 0.614,
		toAvgScore: 0.782,
		fromWhyDistribution: JSON.stringify({
			new_unread: 8200,
			summary_completed: 4200,
			tag_hotspot: 2420,
		}),
		toWhyDistribution: JSON.stringify({
			new_unread: 8400,
			summary_completed: 4412,
			tag_hotspot: 2200,
		}),
	},
};

const RICH_AUDIT_RESULT = {
	...AUDIT_RESULT,
	audit: {
		...AUDIT_RESULT.audit,
		detailsJson: JSON.stringify({
			mismatched_items: ["article:1", "article:2"],
			why_distribution: {
				new_unread: 420,
				summary_completed: 310,
				pulse_need_to_know: 95,
			},
		}),
	},
};

function buildObservabilityWatchResponse() {
	const baseMs = Date.parse("2026-11-27T12:30:00.000Z");
	const makePoints = (values: number[]) =>
		values.map((value, i) => ({
			time: new Date(baseMs + i * 30_000).toISOString(),
			value,
		}));

	return {
		time: "2026-11-27T12:34:56.000Z",
		metrics: [
			{
				key: "http_latency_p95",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				grafanaUrl: "http://127.0.0.1:3000/d/alt-http-latency",
				series: [
					{
						labels: { job: "alt-backend" },
						points: makePoints([0.12, 0.14, 0.19, 0.22, 0.25]),
					},
					{
						labels: { job: "alt-butterfly-facade" },
						points: makePoints([0.08, 0.09, 0.11, 0.12, 0.13]),
					},
					{
						labels: { job: "knowledge-sovereign" },
						points: makePoints([0.04, 0.05, 0.04, 0.06, 0.05]),
					},
				],
			},
			{
				key: "http_rps",
				kind: "SERIES_KIND_RANGE",
				unit: "req/s",
				series: [
					{
						labels: { job: "alt-backend" },
						points: makePoints([12.4, 15.1, 14.8, 16.2, 17.5]),
					},
				],
			},
			{
				key: "availability_services",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [
					{
						labels: { job: "knowledge-sovereign" },
						points: makePoints([1]),
					},
					{
						labels: { job: "alt-butterfly-facade" },
						points: makePoints([1]),
					},
					{
						labels: { job: "recap-subworker" },
						points: makePoints([0]),
					},
				],
			},
		],
	};
}

async function assertNoHorizontalOverflow(page: Page, context: string) {
	const expectedVw = page.viewportSize()?.width ?? 375;
	const report = await page.evaluate((vw) => {
		const issues: string[] = [];

		if (window.innerWidth > vw + 1) {
			issues.push(`window.innerWidth=${window.innerWidth} > expectedVw=${vw}`);
		}
		if (document.documentElement.scrollWidth > vw + 1) {
			issues.push(
				`documentElement.scrollWidth=${document.documentElement.scrollWidth} > vw=${vw}`,
			);
		}
		if (document.body.scrollWidth > vw + 1) {
			issues.push(`body.scrollWidth=${document.body.scrollWidth} > vw=${vw}`);
		}

		const opsPage = document.querySelector<HTMLElement>(
			'[data-role="operations-page"]',
		);
		if (!opsPage) {
			issues.push("missing [data-role=operations-page]");
			return issues;
		}

		const selectors = [
			".ops-header",
			".ops-header-top",
			".ops-actions",
			".ops-tab-section",
			'[data-role="admin-tab-nav"]',
			".ops-section",
			'[data-role="metric-card"]',
			".table-container",
			".panel",
			".percentile-row",
			".audit-meta",
			".diff-grid",
			".diff-dist",
			".observability",
		];

		for (const sel of selectors) {
			for (const el of opsPage.querySelectorAll<HTMLElement>(sel)) {
				const rect = el.getBoundingClientRect();
				if (rect.width === 0 && rect.height === 0) continue;
				if (rect.right > vw + 1 || rect.left < -1) {
					issues.push(
						`${sel} overflow: left=${Math.round(rect.left)} right=${Math.round(rect.right)} vw=${vw}`,
					);
				}
			}
		}

		return issues;
	}, expectedVw);

	expect(report, `Mobile layout overflow on ${context}`).toEqual([]);
}

// Narrowest supported PWA phone viewport (iPhone SE / 375px wide).
test.use({ viewport: { width: 375, height: 667 } });

test.describe("Knowledge Home Admin — Mobile PWA Layout", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(SOVEREIGN_API, (route) => {
			if (route.request().method() === "POST") {
				return fulfillJson(route, RETENTION_RUN_RESULT);
			}
			return fulfillJson(route, SOVEREIGN_ADMIN_SNAPSHOT);
		});

		await page.route(ADMIN_API, (route) => {
			if (route.request().method() === "GET") {
				return fulfillJson(route, RICH_ADMIN_SNAPSHOT);
			}
			const body = route.request().postDataJSON() as { action?: string };
			if (body?.action === "emit_article_url_backfill") {
				return fulfillJson(route, {
					ok: true,
					result: {
						articlesScanned: 27_198,
						eventsAppended: 12_252,
						skippedBlockedScheme: 14_946,
						skippedDuplicate: 0,
						moreRemaining: true,
					},
				});
			}
			if (body?.action === "compare_reproject") {
				return fulfillJson(route, REPROJECT_DIFF_RESPONSE);
			}
			if (body?.action === "run_audit") {
				return fulfillJson(route, RICH_AUDIT_RESULT);
			}
			return fulfillJson(route, { ok: true });
		});

		await page.route(WATCH_URL, (route) =>
			fulfillWatchStream(route, [buildObservabilityWatchResponse()]),
		);
	});

	test("all 7 tabs render without horizontal overflow on 375px PWA viewport", async ({
		page,
	}) => {
		await page.goto("./admin/knowledge-home");

		await expect(
			page.getByRole("heading", { name: "Knowledge Home Operations" }),
		).toBeVisible({ timeout: 10000 });
		await expect(
			page.getByRole("navigation", { name: "Main navigation" }),
		).toBeVisible();

		// Overview tab — including after long Emit URL Backfill result appears in header
		await expect(
			page.getByText("job-running-001").or(page.getByText("75000/150000")),
		).toBeVisible();
		await assertNoHorizontalOverflow(page, "Overview (initial)");

		await page.getByTestId("emit-article-url-backfill-button").click();
		await expect(page.getByTestId("url-backfill-result-summary")).toBeVisible();
		await assertNoHorizontalOverflow(
			page,
			"Overview (with URL backfill summary)",
		);

		// SLO tab
		await page.getByRole("button", { name: "SLO" }).click();
		await expect(
			page.getByText("KnowledgeHomeStreamDisconnectSpike"),
		).toBeVisible();
		await assertNoHorizontalOverflow(page, "SLO tab");

		// System tab
		await page.getByRole("button", { name: "System" }).click();
		await expect(page.getByText("rask-log-aggregator")).toBeVisible();
		await assertNoHorizontalOverflow(page, "System tab");

		// Reproject tab (including Time Range inputs + Compare diff summary)
		await page.getByRole("button", { name: "Reproject" }).click();
		await expect(
			page.getByRole("heading", { name: "Start New Reproject" }),
		).toBeVisible();
		await page.getByLabel("Mode").selectOption("time_range");
		await expect(page.getByLabel("Range Start (RFC3339)")).toBeVisible();
		await page.getByRole("button", { name: "Compare" }).first().click();
		await expect(page.getByText("15,012")).toBeVisible();
		await assertNoHorizontalOverflow(page, "Reproject tab");

		// Inputs on mobile must be >= 16px so iOS Safari PWA does not auto-zoom on focus
		const reprojectInputFontSizes = await page.evaluate(() =>
			Array.from(
				document.querySelectorAll<HTMLInputElement>(
					'[data-role="reproject-actions"] input',
				),
			).map((el) => Number.parseFloat(getComputedStyle(el).fontSize)),
		);
		expect(reprojectInputFontSizes.length).toBeGreaterThan(0);
		for (const size of reprojectInputFontSizes) {
			expect(size).toBeGreaterThanOrEqual(16);
		}

		// Storage tab (including dry-run retention result + snapshot confirmation dialog)
		await page.getByRole("button", { name: "Storage" }).click();
		await expect(page.getByTestId("storage-stats-panel")).toBeVisible();
		await expect(page.getByTestId("retention-status-panel")).toBeVisible();
		await page.getByRole("button", { name: "Run Retention (Dry Run)" }).click();
		await expect(page.locator('[data-role="retention-result"]')).toBeVisible();
		await assertNoHorizontalOverflow(page, "Storage tab");

		await page.getByRole("button", { name: "Create Snapshot" }).click();
		const dialog = page.getByTestId("confirm-action-dialog");
		await expect(dialog).toBeVisible();
		const dialogBox = await dialog.boundingBox();
		expect(dialogBox).not.toBeNull();
		if (!dialogBox) throw new Error("expected dialog bounding box");
		expect(dialogBox.x).toBeGreaterThanOrEqual(0);
		expect(dialogBox.x + dialogBox.width).toBeLessThanOrEqual(376);
		await page.getByRole("button", { name: "Cancel" }).click();
		await expect(dialog).not.toBeVisible();

		// Audit tab (including audit results)
		await page.getByRole("button", { name: "Audit" }).click();
		await expect(page.getByTestId("audit-actions-panel")).toBeVisible();
		await page.getByRole("button", { name: "Run Audit" }).click();
		await expect(page.getByTestId("audit-result-panel")).toBeVisible();
		await assertNoHorizontalOverflow(page, "Audit tab");

		// Observability tab
		await page.getByRole("button", { name: "Observability" }).click();
		await expect(page.getByLabel("Admin observability")).toBeVisible();
		await expect(page.getByText("recap-subworker")).toBeVisible({
			timeout: 10000,
		});
		await assertNoHorizontalOverflow(page, "Observability tab");
	});
});
