import { test, expect, genreLearningBody, triggerBody } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatusIn } from "../../_shared/http.js";
import { eventually } from "../../_shared/eventual.js";
import { readTotalJobs, waitForPipelineIdle } from "../src/assertions.js";
import { env } from "../src/env.js";
import {
	errorSchema,
	genreLearningSchema,
	recapSummarySchema,
	retryAcceptedSchema,
	triggerAcceptedSchema,
} from "../src/schemas.js";

/**
 * The end-to-end pipeline drives — the port of `05-trigger-and-poll-7days.hurl`
 * and `06-trigger-and-poll-3days.hurl`.
 *
 * Each trigger walks the real pipeline: alt-data-hub `ListRecapArticles` →
 * recap-subworker `classify-runs` + clustering → news-creator
 * `summary/generate` → a `recap_outputs` row. Every upstream is
 * `recap-pipeline-stub` under one of its four network aliases, so the work is
 * real and the latency is not.
 *
 * Why this file is serial, and why that is not a cop-out
 * -----------------------------------------------------
 * `Scheduler::run_job` takes a `Semaphore(1)` permit with
 * `try_acquire_owned()` and returns `Err` *immediately* rather than queueing
 * when it cannot get one (scheduler/jobs.rs:186, 214-227) — while the HTTP
 * handler has already answered **202 Accepted** and spawned the task
 * (api/generate.rs:70-84). A concurrent trigger is therefore indistinguishable
 * from a successful one over HTTP, and produces nothing at all.
 *
 * That is not ordering that self-seeding can break: there is one pipeline, one
 * lock, and no request-scoped way to ask for a run of your own. So this file
 * gets a `workers: 1` project (playwright.config.ts) *and* `mode: "serial"`,
 * and every test drains the lock before it ends — because "the 202 came back"
 * is not "the pipeline finished", and the next trigger is the one that would
 * silently evaporate.
 *
 * What is *not* preserved from the Hurl suite is its capture-and-carry: 05
 * captured `job_id` from the trigger and 06 captured its own, and neither ever
 * compared a captured id with the one the fetch returned. Correlating them is
 * not possible from outside anyway — `get_latest_completed_job` returns the
 * newest completed job for a window with no way to ask for a specific one — so
 * what is asserted here is what the reads actually promise: a completed recap
 * for the right window, with every field a real value rather than merely
 * present.
 */

test.describe.configure({ mode: "serial" });

/** Hurl allowed 60×1s for the pipeline; the same budget, asserted not slept. */
const PIPELINE_BUDGET_MS = 120_000;

test.describe("the recap pipeline", () => {
	for (const drive of [
		{ window: "7days", label: "7-day", days: 7 },
		{ window: "3days", label: "3-day", days: 3 },
	] as const) {
		test(`triggering the ${drive.label} window produces a completed recap @slow @contract`, async ({
			api,
		}) => {
			test.setTimeout(PIPELINE_BUDGET_MS + 60_000);

			const before = await readTotalJobs(api);

			// --- the trigger -------------------------------------------------
			const accepted = await expectJsonStatus(
				await api.post(`/v1/generate/recaps/${drive.window}`, {
					data: triggerBody([env.defaultGenre]),
				}),
				202,
				triggerAcceptedSchema,
			);
			expect(accepted.genres).toContain(env.defaultGenre);

			// 202, not 200: the handler spawns `scheduler.run_job` and returns
			// before any work happens (api/generate.rs:83). A 200 would mean
			// somebody made the endpoint synchronous, which would hold a request
			// open for the length of an LLM pipeline.
			//
			// The Hurl suite matched job_id against a UUID regex inline; here it
			// is `uuidSchema` inside `triggerAcceptedSchema`, together with the
			// `status: "accepted"` literal and a non-empty normalized `genres`
			// list — one assertion instead of three, and it fails on a field the
			// spot checks never looked at.

			// --- the read ----------------------------------------------------
			// `--delay`/`retry: 60` becomes a real convergence assertion: a fast
			// stack finishes in one interval, a slow one still passes, and a
			// stuck one fails naming what never became true.
			await eventually(
				async () => {
					const recap = await expectJsonStatus(
						await api.get(`/v1/recaps/${drive.window}`),
						200,
						recapSummarySchema,
					);
					// `window_end - window_start` is the window the scheduler was
					// asked for. The 3-day drive is the whole reason this
					// assertion exists: `get_latest_completed_job` filters on
					// `window_days`, so a `JobContext::new_manual(id, genres, 7)`
					// hard-coded into `trigger_3days` would make the 3-day fetch
					// either 404 forever or hand back the 7-day job — and the
					// Hurl suite, which only checked that four fields existed,
					// would have passed either way.
					const span =
						Date.parse(recap.window_end) - Date.parse(recap.window_start);
					expect(
						Math.round(span / 86_400_000),
						`the ${drive.label} recap must cover ${drive.days} days`,
					).toBe(drive.days);

					// The genre the trigger asked for is the genre that came
					// back. `normalize_genres` lowercases, and `recap_outputs`
					// rows are keyed by genre name, so this is the end-to-end
					// proof that the request reached persistence — not merely
					// that *a* recap exists.
					expect(recap.genres.map((g) => g.genre)).toContain(env.defaultGenre);
				},
				{
					timeout: PIPELINE_BUDGET_MS,
					message: `GET /v1/recaps/${drive.window} serves a completed ${drive.label} recap`,
				},
			);

			// The completed row is written before the run lock is released
			// (status update, then optional classification evaluation, then the
			// permit drops at the end of `run_job`), so the read passing does
			// not mean the next trigger will be accepted. Drain explicitly.
			await waitForPipelineIdle(api, before + 1, PIPELINE_BUDGET_MS);
		});
	}

	test("a trigger with no genres falls back to the configured defaults @slow @contract", async ({
		api,
	}) => {
		test.setTimeout(PIPELINE_BUDGET_MS + 60_000);

		// New coverage. `genres` is `Option<Vec<String>>` with
		// `#[serde(default)]`, and an absent key takes the
		// `state.config().recap_genres()` branch (api/generate.rs:60) rather
		// than the 400 that an all-empty *array* takes
		// (tests/validation.spec.ts). Those two inputs look almost identical
		// from a client and mean opposite things, so both branches need pinning
		// — a handler that treated `None` as "empty" would answer 400 to the
		// nightly batch's own shape, and one that treated `[]` as "None" would
		// run every configured genre for a caller who asked for none.
		//
		// `DEFAULT_GENRE` comes from run.sh, kept in step with the slice's
		// `RECAP_GENRES`, so the compose value and this expectation are one
		// fact rather than two that drift.
		const before = await readTotalJobs(api);

		const accepted = await expectJsonStatus(
			await api.post("/v1/generate/recaps/7days", { data: triggerBody() }),
			202,
			triggerAcceptedSchema,
		);
		expect(accepted.genres).toEqual([env.defaultGenre]);

		// Drained rather than abandoned: the run this started holds the lock,
		// and leaving it in flight is exactly how the next test's trigger would
		// be rejected while still answering 202.
		await waitForPipelineIdle(api, before + 1, PIPELINE_BUDGET_MS);
	});

	test("POST /admin/jobs/retry never reports success for a no-op @contract", async ({ api }) => {
		// New coverage, and the highest-value assertion in this file: it is the
		// regression fence for the rule-8 bug api/admin.rs's own unit tests were
		// written for. The old handler built an empty-`genres` JobContext
		// unconditionally and answered 202 as long as `run_job` returned Ok —
		// "nothing to retry" and "retry started" were the same response.
		//
		// The unit tests pin `retry_outcome_status`; nothing pinned that the
		// *handler* still routes through it, or that the bodies differ. This
		// does, from outside.
		//
		// Three answers are correct, and they are not a shrug — each is a
		// distinct state of `retry_most_recent_failed_job`
		// (scheduler/jobs.rs:244-283):
		//
		//   404  no `failed` recap job exists. The expected answer here: the
		//        drives above completed, so there is nothing to retry.
		//   409  the run lock is held. Reachable if a drive above degraded into
		//        a longer run than its drain observed, or if the JST batch
		//        daemon fired mid-suite (02:00 JST, scheduler/daemon.rs:13).
		//   202  a failed job existed and a fresh run was started. Reachable if
		//        a drive above failed on a genre the stub could not satisfy.
		//
		// Anything else — a 200, or a 202 that cannot name what it is retrying
		// — is the no-op wearing a success status again.
		const response = await api.post("/admin/jobs/retry", { data: {} });
		await expectStatusIn(response, [202, 404, 409]);
		expectHeaderContains(response, "Content-Type", "application/json");

		if (response.status() === 202) {
			const body = await expectJsonStatus(response, 202, retryAcceptedSchema);
			expect(
				body.retried_failed_job_id,
				"a 202 must name the failed job it is retrying, and it must not be the new one",
			).not.toBe(body.job_id);
			// Started a run; hand the lock back before the next test.
			await waitForPipelineIdle(api, 1, PIPELINE_BUDGET_MS);
		} else if (response.status() === 404) {
			const body = await expectJsonStatus(response, 404, errorSchema);
			expect(body.error).toBe("no failed recap job to retry");
		} else {
			const body = await expectJsonStatus(response, 409, errorSchema);
			expect(body.error).toBe("another recap pipeline run is already in progress");
		}
	});

	test("POST /admin/genre-learning persists a graph override @contract", async ({ api }) => {
		// The positive branch of the learning endpoint. It lives here, last in
		// the serial block, rather than with the other `/admin` assertions in
		// tests/validation.spec.ts, because a successful call is not a local
		// write: it inserts a `graph_override` row into `recap_worker_config`,
		// and `PipelineOrchestrator::prepare_pipeline` re-reads the latest such
		// row **before every run** and pushes it into the genre stage
		// (pipeline/orchestrator.rs:429-441, pipeline/graph_override.rs:24-66).
		//
		// Running it in the parallel project would retune the classifier
		// underneath whichever drive above happened to be mid-flight — a
		// cross-test dependency that would surface as an unexplained
		// genre-selection failure minutes later, in a different file.
		//
		// What is asserted is that `config_saved` is not decorative: it is
		// `true` only when `insert_worker_config` returned Ok
		// (api/learning.rs:132-160), so a DB failure cannot present as a
		// successful save.
		const body = await expectJsonStatus(
			await api.post("/admin/genre-learning", {
				data: genreLearningBody({ graph_margin: 0.12, boost_threshold: 0.35 }),
			}),
			200,
			genreLearningSchema,
		);
		expect(body.status).toBe("success");
		expect(body.config_saved).toBe(true);
	});
});
