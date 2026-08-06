import { callUnary } from "../../_shared/connect.js";
import { eventually } from "../../_shared/eventual.js";
import { expectJsonStatus, expectStatusIn } from "../../_shared/http.js";
import { testToken } from "../../_shared/ids.js";
import type { APIRequestContext } from "@playwright/test";
import {
	EVIDENCE_TOPIC,
	P,
	createReport,
	expect,
	fetchReport,
	runStatus,
	startRun,
	test,
	waitForTerminalRun,
} from "../src/fixtures.js";
import {
	getRunStatusResponseSchema,
	listReportVersionsResponseSchema,
	listReportsResponseSchema,
	rerunSectionResponseSchema,
	startReportRunResponseSchema,
} from "../src/schemas.js";

/**
 * Drains a run to a terminal state, tolerating the run row having been deleted.
 *
 * `waitForTerminalRun` cannot be used where a `DeleteReport` may have landed
 * first: `report_runs.report_id` is `ON DELETE CASCADE`
 * (acolyte-migration-atlas/migrations/20260413000000_cascade_delete_reports.sql:24-27),
 * so an accepted delete takes the run row with it and `GetRunStatus` then
 * answers `not_found` forever — which the strict helper would poll through its
 * whole budget before failing. Here a 404 is a legitimate end state, and saying
 * so is what keeps the *real* failure visible: a run wedged in `running`.
 */
async function drainRun(api: APIRequestContext, runId: string): Promise<void> {
	await eventually(
		async () => {
			const response = await callUnary(api, P.getRunStatus, { runId });
			if (response.status() === 404) return;
			const body = await expectJsonStatus(response, 200, getRunStatusResponseSchema);
			expect(["succeeded", "failed", "cancelled"]).toContain(body.run.runStatus);
		},
		{
			timeout: 180_000,
			intervals: [200, 400, 800, 1_500, 3_000],
			message: `run ${runId} either reaches a terminal state or is cascade-deleted`,
		},
	);
}

/**
 * The generation run — the port of `10-start-and-poll-run.hurl` and
 * `11-delete-during-active-run.hurl`.
 *
 * `StartReportRun` creates a `report_runs` row and then hands the work to
 * `asyncio.create_task` (connect_service.py:237-242), so everything after it is
 * eventually consistent. Hurl had one tool for that — `--delay` / `retry` — and
 * `10` spent a 240 × 5000ms budget (a twenty-minute ceiling) waiting for a
 * pipeline that converges in well under a second against the stub. Here the
 * convergence is asserted, not slept through: `waitForTerminalRun` polls the
 * actual condition, so a warm stack finishes on the first probe.
 *
 * Most assertions read a report the worker already drove to completion
 * (`completedRun`, a worker-scoped fixture). That costs one pipeline per worker
 * instead of one per test and, unlike the Hurl chain, it does not make any test
 * depend on another having run first — the fixture *is* the seeding.
 */

test.describe("a run that succeeds", () => {
	// The pipeline drives ten LangGraph nodes, most of them an HTTP round trip
	// to the Ollama stub, and the first test on each worker also pays for the
	// fixture that seeds it. 5 minutes is the ceiling for the whole thing, not
	// an expectation — the stub converges in well under a second.
	test.describe.configure({ timeout: 300_000 });

	test('reaches the terminal status "succeeded" @slow', async ({ completedRun }) => {
		// The exact string is the assertion. `postgres_job_gw.complete_run`
		// writes `run_status = 'succeeded'`; the orchestrator's *log* says
		// "Pipeline completed" (connect_service.py:345). A schema that accepted
		// any string, or an assertion on "is terminal", would let a handler start
		// echoing the log's word and never notice — and every client polling for
		// a terminal state would hang.
		expect(completedRun.terminal.run.runStatus).toBe("succeeded");
		expect(completedRun.terminal.run.runId).toBe(completedRun.runId);
		expect(completedRun.terminal.run.reportId).toBe(completedRun.reportId);

		// target_version_no is `report.current_version + 1` at creation
		// (start_run_uc.py:76), so the very first run of a report targets 1.
		expect(completedRun.terminal.run.targetVersionNo).toBe(1);
		expect(completedRun.terminal.run.failureCode).toBeUndefined();
	});

	test("the status endpoint answers while the run is still in flight @slow", async ({
		acolyte,
	}, testInfo) => {
		// The port of `10`'s single-shot check immediately after StartReportRun,
		// and it catches one specific regression: the background task never
		// starting. `StartReportRun` returns a run id whether or not
		// `asyncio.create_task` fired, so a run row stuck at "pending" forever is
		// indistinguishable from a healthy one until you look.
		//
		// The band is `pending|running|succeeded` because all three are correct
		// this early: the row is created "pending", `mark_running` flips it once
		// the task is scheduled, and against the stub the whole pipeline can be
		// done before this second request lands. What is excluded is "failed" and
		// "cancelled" — neither is reachable one round trip in without something
		// having gone wrong.
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "run-immediate-status"),
			reportType: "market_analysis",
			scope: { topic: EVIDENCE_TOPIC },
		});
		const runId = await startRun(acolyte, reportId);

		const immediate = await runStatus(acolyte, runId);
		expect(immediate.run.runId).toBe(runId);
		expect(immediate.run.reportId).toBe(reportId);
		expect(["pending", "running", "succeeded"]).toContain(immediate.run.runStatus);

		// Drain before finishing so the run does not outlive the test and hold a
		// slot in the `_MAX_CONCURRENT_RUNS = 4` semaphore while other workers
		// are trying to start their own.
		await waitForTerminalRun(acolyte, runId);
	});

	test("the finalizer commits a version and at least one section @slow", async ({
		acolyte,
		completedRun,
	}) => {
		const read = await fetchReport(acolyte, completedRun.reportId);

		expect(read.report.reportId).toBe(completedRun.reportId);
		// Exactly 1, not "at least 1". `completedRun` is a report the fixture
		// created and drove through one run, nothing else writes to it, and
		// `FinalizerNode` is the graph's single terminal node (report_graph.py:204)
		// calling `bump_version` once (finalizer_node.py:98). So one succeeded run
		// leaves current_version at 1. `>= 1` passes just as happily for a finalizer
		// that ran twice, or a `bump_version` whose optimistic-concurrency guard
		// (`WHERE current_version = %s`, postgres_report_gw.py:147-150) stopped
		// refusing a stale write — which is a double-committed version the SPA shows
		// as two identical entries in the history.
		expect(read.report.currentVersion, "one succeeded run commits exactly one version").toBe(1);

		const sections = read.sections ?? [];
		expect(sections.length, "the finalizer committed no sections").toBeGreaterThanOrEqual(1);

		const first = sections[0];
		expect(first).toBeDefined();
		expect(first?.sectionKey.length).toBeGreaterThan(0);

		// New: `citations_json` is a *string* carrying JSON
		// (connect_service.py:102), which is the one field shape a schema alone
		// cannot check. It is the SPA's evidence list; a handler that started
		// writing Python's `repr` of a list — single quotes, `None` — would still
		// be a valid non-empty string and would break `JSON.parse` in the
		// browser. Parsing it here is the only place that fails.
		//
		// Not asserted non-empty: the stub LLM returns generic text rather than
		// acolyte's `<plan>` XML, so whether the writer emits citations depends
		// on prompt behaviour the suite deliberately stays decoupled from.
		for (const section of sections) {
			const citations: unknown = JSON.parse(section.citationsJson);
			expect(
				Array.isArray(citations),
				`citationsJson for section "${section.sectionKey}" is not a JSON array: ` +
					`${section.citationsJson.slice(0, 200)}`,
			).toBe(true);
		}
	});

	test("no active run is surfaced once the run is terminal @slow", async ({
		acolyte,
		completedRun,
	}) => {
		// New coverage for `GetReportResponse.active_run` (acolyte.proto:68,
		// filled at connect_service.py:106-124 from
		// `get_active_run_for_report`, whose SQL filters
		// `run_status IN ('pending','running')`).
		//
		// The field exists so the SPA can resume polling after a reload without
		// remembering a run id — which means a stale `activeRun` on a finished
		// report puts the client into a poll loop that never terminates. The
		// absence is the contract, and it is only assertable from a report whose
		// run is known to have finished.
		const read = await fetchReport(acolyte, completedRun.reportId);
		expect(
			read.activeRun,
			"the run has reached a terminal state, so nothing is in flight",
		).toBeUndefined();
	});

	test("ListReportVersions reflects the finalizer's commit @slow", async ({
		acolyte,
		completedRun,
	}) => {
		const versions = await expectJsonStatus(
			await callUnary(acolyte, P.listReportVersions, {
				reportId: completedRun.reportId,
				limit: 10,
			}),
			200,
			listReportVersionsResponseSchema,
		);

		const list = versions.versions ?? [];
		// One run, one `bump_version`, one `report_versions` INSERT
		// (postgres_report_gw.py:158-166) — so the count is known, not bounded.
		// `>= 1` would report green on the duplicate row a lost optimistic-
		// concurrency guard writes, which is the failure worth catching here:
		// `report_versions` is append-only and a spurious row is not recoverable
		// by re-reading.
		expect(list, "one succeeded run leaves exactly one report_versions row").toHaveLength(1);
		// Ordered `version_no DESC` (postgres_report_gw.py:230), so the head is the
		// newest — and with a single run it is version 1.
		expect(list[0]?.versionNo).toBe(1);

		// The same report answered an empty page before the run — see
		// tests/reports-crud.spec.ts. The pair is what makes this an assertion
		// about the run rather than about the endpoint.
		expect(versions.hasMore, "ten versions were requested and far fewer exist").toBeUndefined();
	});

	test("ListReports carries the run status through to the summary @slow", async ({
		acolyte,
		completedRun,
	}) => {
		// New coverage for connect_service.py:150-154. `ReportSummary` is built
		// by a per-row `get_latest_run_for_report` call rather than a join, so it
		// is a genuinely separate read path from `GetRunStatus` and can drift
		// from it. This is the field a report list renders its status badge from.
		const page = await expectJsonStatus(
			await callUnary(acolyte, P.listReports, { limit: 100 }),
			200,
			listReportsResponseSchema,
		);
		const mine = (page.reports ?? []).find((r) => r.reportId === completedRun.reportId);
		expect(mine, `${completedRun.reportId} should appear in the listing`).toBeDefined();
		expect(mine?.latestRunStatus).toBe("succeeded");
		// The same value `GetReport` reports for this report, and it must be the
		// same number: `ReportSummary` is built from a separate read path
		// (connect_service.py:150-163) and a projection that drifted from
		// `GetReport` is exactly what this test exists to catch. `>= 1` could not
		// see the drift.
		expect(mine?.currentVersion, "one succeeded run commits exactly one version").toBe(1);
	});

	test("RerunSection regenerates a committed section @slow", async ({ acolyte }, testInfo) => {
		// The port of `10`'s final RerunSection entry. It runs against its own
		// report rather than the shared `completedRun` fixture because it
		// *mutates* — `RerunSectionUsecase` bumps the report version — and a
		// fixture other tests read must stay read-only.
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "rerun-section"),
			reportType: "market_analysis",
			scope: { topic: EVIDENCE_TOPIC, dateRange: "2026-04" },
		});
		const runId = await startRun(acolyte, reportId);
		const terminal = await waitForTerminalRun(acolyte, runId);
		expect(terminal.run.runStatus, "the rerun needs a committed section to work from").toBe(
			"succeeded",
		);

		const before = await fetchReport(acolyte, reportId);
		const sectionKey = (before.sections ?? [])[0]?.sectionKey;
		expect(sectionKey, "the run committed no section to rerun").toBeTruthy();

		const response = await callUnary(acolyte, P.rerunSection, { reportId, sectionKey });
		const body = await expectJsonStatus(response, 200, rerunSectionResponseSchema);

		// Strengthened past the Hurl file, which asserted only `HTTP 200`. The
		// handler is synchronous and returns `run_id=""` (connect_service.py:415),
		// which proto3 omits — so the correct response is the empty object. A
		// non-empty `runId` here would mean the rerun became asynchronous, and
		// any client that stopped polling on the strength of the 200 would then
		// be reading a half-written section.
		expect(body.runId, "RerunSection is synchronous and returns no run id").toBeUndefined();

		// And the mutation actually landed: the rerun bumps the report version.
		const after = await fetchReport(acolyte, reportId);
		expect(after.report.currentVersion ?? 0).toBeGreaterThan(before.report.currentVersion ?? 0);
	});
});

test.describe("the active-run guard", () => {
	test.describe.configure({ timeout: 300_000 });

	test("DeleteReport during an active run is refused or the run already finished @slow", async ({
		acolyte,
	}, testInfo) => {
		// The port of `11-delete-during-active-run.hurl`, race and all.
		//
		// `delete_report` calls `has_active_run` (connect_service.py:429), which
		// checks `run_status IN ('pending','running')` and raises
		// FAILED_PRECONDITION. Against the real LLM that window is minutes wide;
		// against the Ollama stub the whole pipeline converges in well under a
		// second, so a delete issued one in-network round trip later can land on
		// either side of the transition. This is the one scenario in the suite
		// that self-seeding cannot make deterministic — the non-determinism is in
		// the service, not in the test's data.
		//
		// The band, member by member:
		//   - **200**  the run reached a terminal state before the delete landed;
		//              there was no active run to guard and the delete is correct.
		//   - **412**  the Connect protocol's status for `failed_precondition`,
		//              which is what `_shared/connect.ts` encodes and what a
		//              spec-conformant server answers.
		//   - **400**  the status the retired Hurl scenario recorded for this
		//              path. connect-python's mapping has never actually been
		//              observed here (the race always resolved to 200 in CI), so
		//              dropping it would be asserting a behaviour nobody has seen.
		// Excluded, and this is what the test is for: any 5xx (the guard blew up
		// on the FK-cascade boundary) and a 404 (the report vanished under a run
		// that is still writing to it).
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "delete-during-run"),
			reportType: "market_analysis",
			scope: { topic: EVIDENCE_TOPIC, dateRange: "2026-04" },
		});
		const runId = await startRun(acolyte, reportId);

		const deletion = await callUnary(acolyte, P.deleteReport, { reportId });
		await expectStatusIn(deletion, [200, 400, 412]);

		// Whichever way it went, the orchestrator must still be answering and the
		// run must not be left in flight: `succeeded` if the delete was refused
		// and the pipeline ran on, `failed` if the delete was accepted and the
		// pipeline then wrote against cascaded-away rows, or gone entirely if the
		// cascade took the run row with the report. A run stuck in `running`
		// forever is the failure this drains for — it would wedge
		// `has_active_run` for that report permanently, which is the state
		// `ReconcileOrphanedRunsUsecase` exists to clean up at the next boot.
		await drainRun(acolyte, runId);
	});

	test("a second run for a report that is already running is refused @slow", async ({
		acolyte,
	}, testInfo) => {
		// New coverage for start_run_uc.py:63-67 — the explicit active-run check
		// that exists because the circuit breaker below it only inspects the most
		// recent *failed* run and would happily let two pipelines write to the
		// same report concurrently. The comment there is candid that this is a
		// best-effort app-layer guard with a TOCTOU window, so the assertion is
		// about the guard firing, not about it being airtight.
		//
		// Same band shape as the delete case above, and for the same reason —
		// 200 means the first run finished before the second request arrived,
		// which is a correct answer rather than a missed guard. 412/400 are the
		// two candidate encodings of `failed_precondition`; a 404 or a 5xx are
		// not correct under any timing.
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "double-start"),
			reportType: "market_analysis",
			scope: { topic: EVIDENCE_TOPIC },
		});

		const firstRunId = await startRun(acolyte, reportId);
		const second = await callUnary(acolyte, P.startReportRun, { reportId });
		await expectStatusIn(second, [200, 400, 412]);

		// Drain whatever exists. When the guard fired there is one run; when the
		// first pipeline had already finished there are two, and leaving the
		// second in flight would hold a slot in the `_MAX_CONCURRENT_RUNS = 4`
		// semaphore while sibling workers are queuing for it.
		await drainRun(acolyte, firstRunId);
		if (second.status() === 200) {
			const accepted = startReportRunResponseSchema.parse(await second.json());
			await drainRun(acolyte, accepted.runId);
		}
	});
});
