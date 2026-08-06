import { test as base } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { callUnary } from "../../_shared/connect.js";
import { eventuallyValue } from "../../_shared/eventual.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { workerToken } from "../../_shared/ids.js";
import { env } from "./env.js";
import {
	createReportResponseSchema,
	getReportResponseSchema,
	getRunStatusResponseSchema,
	startReportRunResponseSchema,
} from "./schemas.js";
import type { GetReportResponse, GetRunStatusResponse } from "./schemas.js";

/**
 * Suite-wide fixtures.
 *
 * The Hurl suite ran with `--jobs 1` because captures are file-scoped and
 * several scenarios consumed a `report_id` a previous entry had produced. That
 * ordering is not reproduced here: every test that reads a report creates it,
 * under a title carrying this worker's token, so four workers can create, list,
 * run and delete concurrently against one acolyte-db without ever seeing each
 * other's rows. Isolation comes from *naming*, not from teardown — the staging
 * slice is destroyed with `docker compose down -v` per dispatch, and a teardown
 * would only buy a race with a sibling worker's `ListReports`.
 */

/** Fully-qualified Connect procedures, as `AcolyteServiceASGIApplication` mounts them. */
export const P = {
	createReport: "alt.acolyte.v1.AcolyteService/CreateReport",
	getReport: "alt.acolyte.v1.AcolyteService/GetReport",
	listReports: "alt.acolyte.v1.AcolyteService/ListReports",
	getReportVersion: "alt.acolyte.v1.AcolyteService/GetReportVersion",
	listReportVersions: "alt.acolyte.v1.AcolyteService/ListReportVersions",
	diffReportVersions: "alt.acolyte.v1.AcolyteService/DiffReportVersions",
	startReportRun: "alt.acolyte.v1.AcolyteService/StartReportRun",
	getRunStatus: "alt.acolyte.v1.AcolyteService/GetRunStatus",
	streamRunProgress: "alt.acolyte.v1.AcolyteService/StreamRunProgress",
	rerunSection: "alt.acolyte.v1.AcolyteService/RerunSection",
	deleteReport: "alt.acolyte.v1.AcolyteService/DeleteReport",
	healthCheck: "alt.acolyte.v1.AcolyteService/HealthCheck",
} as const;

/**
 * The topic the seeded Meilisearch corpus actually answers to.
 *
 * `e2e/fixtures/search-indexer/seed-docs.json` carries four "AI infrastructure
 * trends…" documents; the gatherer node queries search-indexer with the brief's
 * topic, so using anything else here would silently exercise the empty-evidence
 * branch of the pipeline instead of the one production takes.
 */
export const EVIDENCE_TOPIC = "AI infrastructure trends";

export type ReportInput = {
	readonly title: string;
	readonly reportType: string;
	readonly scope?: Record<string, string>;
};

/** A report driven all the way through the LangGraph pipeline to a terminal run. */
export type CompletedRun = {
	readonly reportId: string;
	readonly runId: string;
	readonly terminal: GetRunStatusResponse;
};

// ---------------------------------------------------------------------------
// Request helpers
// ---------------------------------------------------------------------------

/** `CreateReport`, asserting the envelope and handing back the id. */
export async function createReport(
	api: APIRequestContext,
	input: ReportInput,
): Promise<string> {
	const body = await expectJsonStatus(
		await callUnary(api, P.createReport, input),
		200,
		createReportResponseSchema,
	);
	return body.reportId;
}

export async function fetchReport(
	api: APIRequestContext,
	reportId: string,
): Promise<GetReportResponse> {
	return expectJsonStatus(
		await callUnary(api, P.getReport, { reportId }),
		200,
		getReportResponseSchema,
	);
}

export async function startRun(api: APIRequestContext, reportId: string): Promise<string> {
	const body = await expectJsonStatus(
		await callUnary(api, P.startReportRun, { reportId }),
		200,
		startReportRunResponseSchema,
	);
	return body.runId;
}

export async function runStatus(
	api: APIRequestContext,
	runId: string,
): Promise<GetRunStatusResponse> {
	return expectJsonStatus(
		await callUnary(api, P.getRunStatus, { runId }),
		200,
		getRunStatusResponseSchema,
	);
}

/**
 * Polls `GetRunStatus` until the run leaves pending/running.
 *
 * The Hurl file this replaces slept its way there: `retry: 240` at
 * `retry-interval: 5000`, a twenty-minute ceiling sized for a real LLM, paid in
 * five-second granularity even though the stub converges in well under a
 * second. `eventuallyValue` asserts the condition instead, so a fast stack
 * finishes on the first 200ms probe and a cold one still passes.
 *
 * Deliberately terminal-*any*, not `succeeded`: a run that has already failed
 * must stop the loop at once, so a genuine pipeline break surfaces in seconds
 * rather than after the whole budget has drained. The strict "and it was
 * succeeded" assertion is a separate step, exactly as it was in Hurl.
 */
export async function waitForTerminalRun(
	api: APIRequestContext,
	runId: string,
): Promise<GetRunStatusResponse> {
	await eventuallyValue(
		async () => (await runStatus(api, runId)).run.runStatus,
		`run ${runId} leaves pending/running for a terminal state`,
		{ timeout: 180_000, intervals: [200, 400, 800, 1_500, 3_000] },
	).toMatch(/^(succeeded|failed|cancelled)$/);

	return runStatus(api, runId);
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

type WorkerFixtures = {
	/** Unique to (dispatch, worker). Embedded in every title this worker creates. */
	workerTag: string;
	/** Connect-RPC client for :8090, JSON codec. */
	acolyte: APIRequestContext;
	/** Same listener, no Connect headers — for `GET /health` and route negatives. */
	rest: APIRequestContext;
	/** A report this worker drove to a terminal run, for the read-only run assertions. */
	completedRun: CompletedRun;
};

export const test = base.extend<Record<never, never>, WorkerFixtures>({
	workerTag: [
		async ({}, use, workerInfo) => {
			await use(workerToken(workerInfo.workerIndex));
		},
		{ scope: "worker" },
	],

	acolyte: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: {
					"Content-Type": "application/json",
					// Sent on every call because the Hurl suite sent it on every
					// call: whether connect-python *requires* it is not something
					// this suite is asserting, and silently dropping it would
					// change the protocol under test rather than testing it.
					"Connect-Protocol-Version": "1",
				},
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	rest: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.baseURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	completedRun: [
		async ({ acolyte, workerTag }, use) => {
			const reportId = await createReport(acolyte, {
				title: `acolyte-e2e-${workerTag}-completed`,
				reportType: "market_analysis",
				scope: { topic: EVIDENCE_TOPIC, dateRange: "2026-04" },
			});
			const runId = await startRun(acolyte, reportId);
			const terminal = await waitForTerminalRun(acolyte, runId);

			if (terminal.run.runStatus !== "succeeded") {
				// Fail here rather than letting five downstream assertions each
				// report a missing `sections` array. The failure code is the
				// only thing that says *which* node gave up, so it goes in the
				// message: `no_evidence` means search-indexer returned nothing
				// (check the Meilisearch seed in global setup), `no_content`
				// means the hydrator/compressor chain produced nothing
				// groundable, `pipeline_crashed` means an exception escaped
				// (see report_graph and the orchestrator container log).
				throw new Error(
					`seeding a completed run failed: run ${runId} for report ${reportId} ` +
						`ended as "${terminal.run.runStatus}" ` +
						`(failureCode=${terminal.run.failureCode ?? "<none>"}, ` +
						`failureMessage=${terminal.run.failureMessage ?? "<none>"})`,
				);
			}

			// No teardown: the slice is destroyed per dispatch, and deleting the
			// report here would race tests/reports-list.spec.ts, which lists
			// every report in the database.
			await use({ reportId, runId, terminal });
		},
		{ scope: "worker" },
	],
});

export { expect } from "@playwright/test";
