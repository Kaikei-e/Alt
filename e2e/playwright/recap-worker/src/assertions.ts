import { expect } from "@playwright/test";
import type { APIRequestContext, APIResponse } from "@playwright/test";
import { expectJsonStatus } from "../../_shared/http.js";
import { eventually } from "../../_shared/eventual.js";
import { jobStatsSchema } from "./schemas.js";

/**
 * Assertions specific to an axum service, and to this one's pipeline lock.
 *
 * `_shared/` has `expectProcedureMounted` for Connect-RPC and
 * `expectPrometheusText` for a working exporter. recap-worker needs neither
 * shape — it speaks plain REST and its exporter is currently mute — so the two
 * equivalents live here. Both are candidates for `_shared/` once a second
 * axum suite wants them; see this suite's README.
 */

// ---------------------------------------------------------------------------
// Route mounting
// ---------------------------------------------------------------------------

/**
 * Asserts a route is **mounted at this method**, by discriminating 404 and 405
 * from every other answer.
 *
 * This is `_shared/connect.ts`'s `expectProcedureMounted` translated to axum,
 * and it is CLAUDE.md rule 8 at the E2E boundary. `api::router` is one long
 * builder chain of `.route(path, get(h))` / `.route(path, post(h))` calls
 * (api.rs:17-70); a line dropped in a rebase makes the endpoint answer 404,
 * and a `get`/`post` swapped by autocomplete makes it answer 405. Neither is
 * visible to a unit test — the handler function still compiles and still has
 * its own tests — and neither is distinguishable from a business outcome by
 * any assertion of the form "not 2xx".
 *
 * `expected` is what a correctly wired route answers for *this* request. For a
 * probe that must not do real work, the cheapest choice is a deliberately
 * malformed request: axum's extractors reject before the handler body runs, so
 * 415 from `Json`'s missing-content-type branch proves the route resolved
 * without triggering the pipeline behind it.
 */
export async function expectRouteMounted(
	response: APIResponse,
	expected: readonly number[],
): Promise<void> {
	const body = (await response.text()).slice(0, 500);
	expect(
		response.status(),
		`${response.url()} answered 404. The route was never registered — check the ` +
			`.route(...) chain in recap-worker/src/api.rs, not the request.\n${body}`,
	).not.toBe(404);
	expect(
		response.status(),
		`${response.url()} answered 405. The path is registered but not for this ` +
			`method — a get()/post() swap in api.rs.\nAllow: ${response.headers()["allow"]}`,
	).not.toBe(405);
	expect(
		expected,
		`${response.url()} answered ${response.status()}\n${body}`,
	).toContain(response.status());
}

// ---------------------------------------------------------------------------
// Prometheus
// ---------------------------------------------------------------------------

/**
 * A line that Prometheus' text exposition format admits: a comment
 * (`# HELP` / `# TYPE`) or `name{labels} value [timestamp]`.
 */
const EXPOSITION_LINE = /^(#|[a-zA-Z_:][a-zA-Z0-9_:]*(\{.*\})?\s)/;

/**
 * Asserts `/metrics` serves the exposition format — without requiring it to
 * be non-empty.
 *
 * `_shared/http.ts`'s `expectPrometheusText` demands a `# HELP` line, which is
 * the right assertion for a service whose exporter works. recap-worker's does
 * not, and the reason is a real, known bug rather than a gap in this suite:
 * `Metrics::new` registers all sixty-odd families into a freshly constructed
 * `Registry` (observability/metrics.rs:68+, observability.rs:23-24), while
 * `Telemetry::render_prometheus` encodes `prometheus::gather()` — the process
 * -wide *default* registry, which nothing in this service ever writes to
 * (observability.rs:76-82). The endpoint therefore answers 200 with an empty
 * body and every recap dashboard stays blank while the scrape reports success.
 *
 * Asserting "200, text/plain, and every non-empty line is valid exposition"
 * pins the envelope the scraper depends on and stays true either way: the day
 * the registry binding is fixed, the families appear and this still passes.
 * The stronger family-name assertion belongs in the same change that fixes it.
 */
export function expectPrometheusExposition(body: string, url: string): void {
	expect(
		body.startsWith("{"),
		`${url} served a JSON body, which means the handler fell through to an ` +
			`error envelope instead of the text encoder: ${body.slice(0, 300)}`,
	).toBe(false);

	for (const line of body.split("\n")) {
		if (line.trim() === "") continue;
		expect(line, `${url} served a line that is not Prometheus exposition`).toMatch(
			EXPOSITION_LINE,
		);
	}
}

// ---------------------------------------------------------------------------
// The pipeline run lock
// ---------------------------------------------------------------------------

/**
 * Waits until no recap pipeline is in flight and at least `minTotalJobs` jobs
 * have been kicked in the last 24h.
 *
 * `Scheduler::run_job` takes a `Semaphore(1)` permit with `try_acquire_owned`
 * and returns `Err` immediately if it cannot (scheduler/jobs.rs:214-227) — but
 * the HTTP handler has already answered **202 and spawned the task**
 * (api/generate.rs:70-84), so the rejection is only ever a log line. A second
 * trigger issued while the first run holds the permit therefore looks
 * completely successful over HTTP and produces nothing at all.
 *
 * That is why the pipeline specs run in a single-worker project, and why each
 * of them ends here: "the previous test's 202 was answered" is not "the
 * previous test's pipeline finished", and without the drain the next trigger
 * is the one that silently evaporates.
 *
 * `minTotalJobs` closes the other half of the race. Immediately after a 202,
 * `running_jobs` is still 0 because the pipeline has not inserted its
 * `recap_jobs` row yet — so waiting on `running_jobs === 0` alone would return
 * instantly and prove nothing. Requiring the total to have grown first makes
 * the wait observe the row's whole life.
 */
export async function waitForPipelineIdle(
	api: APIRequestContext,
	minTotalJobs: number,
	timeout = 120_000,
): Promise<void> {
	await eventually(
		async () => {
			const stats = await expectJsonStatus(
				await api.get("/v1/dashboard/job-stats"),
				200,
				jobStatsSchema,
			);
			expect(stats.total_jobs_24h).toBeGreaterThanOrEqual(minTotalJobs);
			expect(stats.running_jobs).toBe(0);
		},
		{
			timeout,
			message:
				`the recap pipeline goes idle with at least ${minTotalJobs} job(s) kicked ` +
				`in the last 24h (Scheduler's run_lock permit is released)`,
		},
	);
}

/** Current `total_jobs_24h`, for sizing the next `waitForPipelineIdle`. */
export async function readTotalJobs(api: APIRequestContext): Promise<number> {
	const stats = await expectJsonStatus(
		await api.get("/v1/dashboard/job-stats"),
		200,
		jobStatsSchema,
	);
	return stats.total_jobs_24h;
}
