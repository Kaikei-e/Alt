import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { errorSchema, jobStatsSchema } from "../src/schemas.js";

/**
 * What recap-worker answers before anything has ever run — the port of
 * `03-recaps-empty.hurl`, widened.
 *
 * Why this file is its own single-worker project
 * ----------------------------------------------
 * Every assertion here is about the *absence* of a row, and recap-worker
 * offers no way for a test to own an absence. `get_latest_completed_job`
 * selects the newest completed job for a window with no tenant, owner or name
 * predicate (api/fetch.rs:404-418); `get_latest_morning_letter`,
 * `get_latest_pulse` and `get_latest_genre_evaluation` are the same shape.
 * There is no token a test could seed under to carve out its own empty
 * database, so these are observable exactly once per stack — which is what the
 * Hurl suite's `--jobs 1` was really buying, and all it was buying.
 *
 * `playwright.config.ts` gives this file a `workers: 1` project that both
 * other projects declare as a dependency, so it runs to completion before the
 * first trigger is issued and the other thirty-odd tests still run in
 * parallel. If one of these ever needs to move, the honest fix is a
 * per-request scoping key on the read endpoints, not a `serial` describe.
 *
 * The empty-database *envelopes* that a unique identifier CAN isolate — a
 * letter for 2020-01-01, an evaluation run under a fresh UUID, a search for a
 * term nothing indexes — deliberately live in tests/validation.spec.ts and
 * tests/read-surface.spec.ts instead, where they run in parallel and stay
 * true for the whole life of the stack.
 */

test.describe("an empty recap_db", () => {
	test("GET /v1/recaps/7days answers 404 with the 7-day miss envelope @contract", async ({
		api,
	}) => {
		// api/fetch.rs:407-418 — Ok(None) becomes
		// (NOT_FOUND, Json(ErrorResponse { error: "No {label} recap found" })).
		// The exact string matters: alt-backend's recap gateway treats this 404
		// as "nothing to show yet" and anything else as an upstream fault, so a
		// handler that started returning 200 + `{"genres": []}` here would put
		// an empty recap in front of a user instead of the empty state.
		const response = await api.get("/v1/recaps/7days");
		const body = await expectJsonStatus(response, 404, errorSchema);
		expect(body.error).toBe("No 7-day recap found");
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("GET /v1/recaps/3days answers 404 with the 3-day miss envelope @contract", async ({
		api,
	}) => {
		// The label is interpolated from the caller (`get_3days_recap` passes
		// "3-day"), so this is not a copy of the test above: it is the assertion
		// that the two windows do not share a message — which is how a
		// `get_recap_by_window(state, 3, "7-day")` copy-paste would be caught.
		const response = await api.get("/v1/recaps/3days");
		const body = await expectJsonStatus(response, 404, errorSchema);
		expect(body.error).toBe("No 3-day recap found");
	});

	test("GET /v1/morning/letters/latest answers 404 before any letter exists @contract", async ({
		api,
	}) => {
		// New coverage; the Hurl suite listed the morning surface as "out of
		// scope (deferred phases)". api/fetch.rs:893-899.
		const body = await expectJsonStatus(
			await api.get("/v1/morning/letters/latest"),
			404,
			errorSchema,
		);
		expect(body.error).toBe("No morning letter found");
	});

	test("GET /v1/pulse/latest answers 404 naming today when no pulse exists @contract", async ({
		api,
	}) => {
		// New coverage. api/pulse.rs:146-163 interpolates the *requested* date
		// into the message, falling back to the literal "today" when the query
		// omits `date`. tests/validation.spec.ts asserts the dated branch, which
		// is order-independent; this one pins the default branch, which is not.
		const body = await expectJsonStatus(await api.get("/v1/pulse/latest"), 404, errorSchema);
		expect(body.error).toBe("No evening pulse found for date today");
	});

	test("GET /v1/evaluation/genres/latest answers 404 before any run @contract", async ({
		api,
	}) => {
		// New coverage. api/evaluation.rs:1120-1125.
		const body = await expectJsonStatus(
			await api.get("/v1/evaluation/genres/latest"),
			404,
			errorSchema,
		);
		expect(body.error).toBe("No evaluation results found");
	});

	test("GET /v1/dashboard/job-stats starts from an all-zero ledger @contract", async ({
		api,
	}) => {
		// New coverage, and the one assertion here that is about a 200 rather
		// than a 404. It is worth having for two reasons.
		//
		// First, it proves the aggregate in store/dao/job_status.rs:105-119 is
		// reading a table that exists and is empty, rather than silently
		// counting rows the migrator left behind — the failure mode `run.sh`'s
		// `down -v` exists to prevent, and which would otherwise only surface as
		// a confusing recap in the middle of a later spec.
		//
		// Second, `success_rate` is `COUNT(*) FILTER (…)/NULLIF(COUNT(*),0)`,
		// which is SQL NULL on an empty table; the DAO maps that to 0.0
		// (job_status.rs:128). Without the NULLIF the query divides by zero, and
		// without the unwrap the field would serialise as `null` and break every
		// dashboard consumer. Zero jobs is the only state in which either bug is
		// reachable.
		const stats = await expectJsonStatus(
			await api.get("/v1/dashboard/job-stats"),
			200,
			jobStatsSchema,
		);
		expect(stats.total_jobs_24h).toBe(0);
		expect(stats.running_jobs).toBe(0);
		expect(stats.failed_jobs_24h).toBe(0);
		expect(stats.success_rate_24h).toBe(0);
		expect(stats.avg_duration_secs).toBeNull();
	});
});
