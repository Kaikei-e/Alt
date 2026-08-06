import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { z } from "zod";
import {
	adminJobSchema,
	jobProgressSchema,
	jobStatsSchema,
	logErrorSchema,
	recapJobSchema,
	recentActivitySchema,
	systemMetricSchema,
} from "../src/schemas.js";

/**
 * The operator dashboard — entirely new coverage.
 *
 * Seven routes (`api.rs:61-69`) that the Hurl suite never touched. They are
 * worth a spec for a reason beyond completeness: every one of them is a
 * `Result<Json<T>, (StatusCode, String)>` handler whose error arm stringifies
 * the sqlx error straight into the response body
 * (`e.to_string()`, api/dashboard.rs:85, 114, 143, 176, 212, 391…). A missing
 * column or a renamed table therefore produces a 500 carrying the SQL, and
 * nothing else in the build notices — these queries are hand-written strings,
 * invisible to the compiler and to every unit test that uses the mock DAO.
 *
 * So the assertion that earns its place here is the cheapest one: each route
 * answers 200 with rows of the shape its struct declares. That is a live
 * schema check against the migrated database.
 *
 * All of it is population-independent, so it runs in parallel alongside the
 * pipeline project writing rows underneath it.
 */

/** Every list endpoint takes the same `?window=<secs>&limit=<n>` pair. */
const LIST_ROUTES = [
	{ path: "/v1/dashboard/metrics", schema: systemMetricSchema, defaultLimit: 500 },
	{ path: "/v1/dashboard/overview", schema: recentActivitySchema, defaultLimit: 200 },
	{ path: "/v1/dashboard/logs", schema: logErrorSchema, defaultLimit: 2_000 },
	{ path: "/v1/dashboard/jobs", schema: adminJobSchema, defaultLimit: 200 },
	{ path: "/v1/dashboard/recap_jobs", schema: recapJobSchema, defaultLimit: 200 },
] as const;

test.describe("dashboard list endpoints", () => {
	for (const route of LIST_ROUTES) {
		test(`GET ${route.path} answers rows of the declared shape @contract`, async ({ api }) => {
			// The schema is the point. A bare "is an array" check passes for the
			// `[]` a broken query returns after the handler's `?` swallowed
			// nothing — but these handlers do not swallow, they 500, so the real
			// failure this catches is a *column* rename: sqlx's `try_get` fails
			// per row, the handler maps it to 500, and this test reports the SQL
			// error text in its failure message via expectJsonStatus's preview.
			await expectJsonStatus(await api.get(route.path), 200, z.array(route.schema));
		});

		test(`GET ${route.path} honours limit @contract (default ${route.defaultLimit})`, async ({
			api,
		}) => {
			// `params.limit.unwrap_or(<default>)` is passed straight into the
			// query's LIMIT — the defaults differ per route (500 for metrics,
			// 2000 for logs, 200 for the rest), which is why they are carried in
			// the table above and named in the title. Asking for 1 is checkable
			// at any population, including zero rows, and is what would catch the
			// limit being dropped from the SQL — which nothing else here would
			// notice, because an unbounded result is still a well-shaped one.
			const body = await expectJsonStatus(
				await api.get(`${route.path}?limit=1`),
				200,
				z.array(route.schema),
			);
			expect(body.length).toBeLessThanOrEqual(1);
		});

		test(`GET ${route.path} rejects a non-numeric window @contract`, async ({ api }) => {
			// `window: Option<i64>`, so serde_urlencoded fails the struct and
			// axum's `Query` extractor answers 400 before the handler builds a
			// query. Worth pinning per route because a "be forgiving" change to
			// any one of them would let a typo'd window silently become the
			// 14400s default and quietly narrow an operator's investigation.
			await expectStatus(await api.get(`${route.path}?window=abc`), 400);
		});
	}
});

test.describe("job progress", () => {
	test("GET /v1/dashboard/job-progress answers the full progress envelope @contract", async ({
		api,
	}) => {
		// `JobProgressEvent` is four fields — `active_job`, `recent_jobs`,
		// `stats`, `user_context` — assembled from five separate DAO calls
		// (api/dashboard.rs:379-449). Two of them are nullable, and the schema
		// says so rather than requiring them: `active_job` is null unless a
		// pipeline holds the run lock, which depends on whether the `pipeline`
		// project happens to be mid-drive right now.
		//
		// The invariant that does hold at any moment is that `stats` and
		// `recent_jobs` are always present — they come from unconditional
		// queries — so a null there means a handler started tolerating a DAO
		// failure it currently propagates.
		const body = await expectJsonStatus(
			await api.get("/v1/dashboard/job-progress"),
			200,
			jobProgressSchema,
		);
		expect(body.user_context, "user_context is populated only when user_id is supplied").toBeNull();
	});

	test("job-progress builds a user context only when asked @contract", async ({ api, seed }) => {
		// `params.user_id` gates both the recent-jobs query
		// (`get_user_jobs` vs `get_extended_jobs`) and the `user_context` block
		// (api/dashboard.rs:400-441). A UUID no job carries must therefore
		// produce a *present* context whose counters are zero — not a null
		// context, and not somebody else's jobs.
		//
		// The absent-user case is what makes this parallel-safe: a fresh v4 UUID
		// can never collide with the system-triggered jobs the pipeline project
		// creates, which carry `user_id = NULL`.
		const body = await expectJsonStatus(
			await api.get(`/v1/dashboard/job-progress?user_id=${seed.absentId}`),
			200,
			jobProgressSchema,
		);
		expect(body.user_context).not.toBeNull();
		expect(body.user_context?.user_jobs_count).toBe(0);
		expect(body.recent_jobs).toEqual([]);
	});

	test("job-progress rejects a malformed user_id @contract", async ({ api }) => {
		// `user_id: Option<Uuid>` in `JobProgressQuery` (api/dashboard.rs:245).
		// A 200 here would mean the field had been loosened to a string and the
		// UUID parse moved somewhere that swallows failures — at which point
		// "show me this user's jobs" quietly becomes "show me everyone's".
		await expectStatus(await api.get("/v1/dashboard/job-progress?user_id=not-a-uuid"), 400);
	});
});

test.describe("job stats", () => {
	test("GET /v1/dashboard/job-stats answers the aggregate envelope @contract", async ({
		api,
	}) => {
		// The population-independent half; tests/cold-start.spec.ts asserts the
		// all-zero values that are only observable before anything runs.
		//
		// `success_rate_24h` is `COUNT(*) FILTER (WHERE status='completed') /
		// NULLIF(COUNT(*),0)` mapped through `unwrap_or(0.0)`
		// (store/dao/job_status.rs:105-128). It is a ratio, so it is bounded —
		// and a value outside [0,1] means the numerator and denominator stopped
		// counting the same population, which is the kind of drift that makes an
		// operator dashboard confidently wrong.
		const stats = await expectJsonStatus(
			await api.get("/v1/dashboard/job-stats"),
			200,
			jobStatsSchema,
		);
		expect(stats.success_rate_24h).toBeGreaterThanOrEqual(0);
		expect(stats.success_rate_24h).toBeLessThanOrEqual(1);
		expect(stats.running_jobs).toBeLessThanOrEqual(stats.total_jobs_24h);
		expect(stats.failed_jobs_24h).toBeLessThanOrEqual(stats.total_jobs_24h);
	});
});
