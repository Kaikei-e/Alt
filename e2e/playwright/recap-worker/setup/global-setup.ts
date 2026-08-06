import { httpBody, httpOk, waitForReady } from "../../_shared/readiness.js";
import { env } from "../src/env.js";

/**
 * Readiness gate — the direct replacement for `00-setup.hurl`.
 *
 * The Hurl suite opened with a 30×500ms retry on `/health/live` and nothing
 * else, because `/health/live` was the probe it was about to validate in
 * scenario 01 and gating on the thing under test felt circular. That reasoning
 * does not survive the move: a readiness check that lives *inside* the suite is
 * order-dependent by construction, and `fullyParallel` has no notion of "run
 * this one first". Here it runs once, before any worker starts, and a stack
 * that never comes up fails with one legible message naming the probe that
 * never passed — instead of failing thirty tests with a connection error and
 * leaving the reader to work out which one was the cause.
 *
 * Gating on more than liveness is the other half. `run.sh` waits on compose
 * healthchecks, but recap-worker's healthcheck is
 * `["CMD", "/usr/local/bin/recap-worker", "healthcheck"]` — a self-probe that
 * proves the listener bound, and nothing about the four upstreams or the
 * database. Three things can still be settling at that moment, and each has
 * its own probe below.
 */
export default async function globalSetup(): Promise<void> {
	await waitForReady([
		/**
		 * 1. The listener answers and the axum router is mounted.
		 *
		 * Dependency-free by construction: `health::live` touches nothing but a
		 * tracing call (api/health.rs:55-61). If this never passes, the process
		 * is not serving — look at the container, not at the stack.
		 */
		httpBody(
			`${env.baseURL}/health/live`,
			(body) =>
				typeof body === "object" &&
				body !== null &&
				(body as Record<string, unknown>)["status"] === "live",
			`GET ${env.baseURL}/health/live reports status=live`,
		),

		/**
		 * 2. Both HTTP upstreams answer.
		 *
		 * `health::ready` calls `subworker_client.ping()` and
		 * `news_creator_client.health_check()` in sequence and turns either
		 * failure into a 503 with `{"status":"degraded", …}`
		 * (api/health.rs:31-53). Both resolve to `recap-pipeline-stub` through
		 * its `rw-stub-subworker` / `rw-stub-news-creator` aliases, and the stub
		 * is a uvicorn process whose first import takes a few seconds — so this
		 * is the probe that actually waits for the slice, rather than for the
		 * container.
		 *
		 * Checking `status === "ready"` rather than `response.ok()` matters: the
		 * degraded answer is a 503, but a future "report degraded but keep
		 * serving" change would make it a 200 and `httpOk` would sail past it.
		 */
		httpBody(
			`${env.baseURL}/health/ready`,
			(body) =>
				typeof body === "object" &&
				body !== null &&
				(body as Record<string, unknown>)["status"] === "ready",
			`GET ${env.baseURL}/health/ready reports status=ready (stubbed subworker + news-creator both answer)`,
		),

		/**
		 * 3. The database is reachable and migrated.
		 *
		 * Nothing above touches Postgres: `ComponentRegistry::build` uses
		 * `connect_lazy` (app.rs:120-128), so the pool opens no connection until
		 * the first query, and both health probes are query-free. Meanwhile
		 * `recap-db-migrator` is a one-shot container that `run.sh` deliberately
		 * does not `--wait` on (it exits, and `--wait` would poll it forever);
		 * recap-worker's `service_completed_successfully` gate is what orders it.
		 *
		 * `/v1/dashboard/job-stats` is the cheapest endpoint that issues a real
		 * query — a single aggregate over `recap_jobs`
		 * (store/dao/job_status.rs:105-119). A 200 here means the pool
		 * connected, the schema exists and Atlas ran. Without this probe, a
		 * migration failure surfaces as a 500 on whichever DB-backed spec
		 * happened to run first.
		 */
		httpOk(
			`${env.baseURL}/v1/dashboard/job-stats`,
			`GET ${env.baseURL}/v1/dashboard/job-stats answers 200 (recap_db pool connected, Atlas migrations applied)`,
		),
	]);
}
