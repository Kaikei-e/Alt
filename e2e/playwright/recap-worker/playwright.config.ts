import { defineApiSuite } from "../_shared/config.js";

/**
 * recap-worker API E2E.
 *
 * One plaintext axum listener on :9005 (`RECAP_WORKER_HTTP_BIND`), no
 * Connect-RPC server surface of its own — recap-worker is a Connect *client*
 * of alt-data-hub, not a host — and a second rustls listener on :9443 that
 * this slice must leave unbound. HTTP only; no spec touches `page`, `browser`
 * or `context`.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop
 * lives in `defineApiSuite` (see `_shared/config.ts`). What is genuinely local
 * to recap-worker is the project layout below, and it is unusual for this
 * fleet, so it is worth reading before adding a spec.
 */
export default defineApiSuite({
	service: "recap-worker",

	/**
	 * Sized against the **database pool**, which is the only shared resource
	 * the read surface contends for: `RECAP_DB_MAX_CONNECTIONS` defaults to 50
	 * (config.rs:1029) and the staging slice does not override it, against a
	 * stock `postgres` with `max_connections=100`.
	 *
	 * So the pool is nowhere near the ceiling and 4 is a deliberate
	 * under-shoot, for a different reason: recap-worker has no tenant
	 * dimension and no per-caller partitioning, so most of what these specs
	 * read is one shared table. Extra workers buy wall-clock on the validation
	 * and topology specs — which is where the test count is — and buy nothing
	 * on the rest, while adding contention to a single tokio runtime that is
	 * also, in the `pipeline` project, running a real ML pipeline. 4 is the
	 * point where the parallel projects stop being the long pole.
	 */
	workers: 4,

	/**
	 * 45s rather than the 30s default. `connect_lazy` + `test_before_acquire`
	 * (app.rs:120-127) means the very first DB-backed request in a worker pays
	 * for opening the connection, and the staging Postgres is cold. The
	 * genuinely long tests — the pipeline drives — set their own budget with
	 * `test.setTimeout`, so this is headroom, not a blanket licence.
	 */
	timeout: 45_000,

	globalSetup: "./setup/global-setup.ts",

	/**
	 * Three projects, because recap-worker has two kinds of state that naming
	 * cannot isolate, and they need opposite treatment.
	 *
	 * `cold-start` — the empty-database observations. `GET /v1/recaps/7days`
	 * answers 404 only while `recap_outputs` holds no completed job for that
	 * window (api/fetch.rs:407-418), and there is no tenant, prefix or token
	 * that would let a test carve out its own "empty". It is a genuine global
	 * singleton observable exactly once per stack, so it gets a single-worker
	 * project that every other project `dependsOn`. The Hurl suite expressed
	 * the same constraint as `--jobs 1` over the *whole* suite; this expresses
	 * it over the four scenarios that actually need it and lets the other
	 * thirty run in parallel.
	 *
	 * `pipeline` — the trigger-and-poll drives. `Scheduler::run_job` holds a
	 * `Semaphore(1)` and *rejects* rather than queues a concurrent run
	 * (scheduler/jobs.rs:186, 214-227), while the HTTP handler has already
	 * returned 202 and spawned the task (api/generate.rs:70-84). Two
	 * overlapping triggers therefore both look successful and only one
	 * produces anything — the exact shape of failure that makes a parallel
	 * suite flaky for reasons no failure message explains. One worker, and
	 * `mode: "serial"` inside the file, with each test draining the lock
	 * before it ends.
	 *
	 * `api` — everything else. Health, the metrics envelope, every validation
	 * and path-parameter negative, the read surface, the dashboard, topology.
	 * None of it writes anything another test reads.
	 */
	projects: [
		{
			name: "cold-start",
			testMatch: /cold-start\.spec\.ts$/,
			workers: 1,
		},
		{
			name: "api",
			testIgnore: [/cold-start\.spec\.ts$/, /pipeline\.spec\.ts$/],
			dependencies: ["cold-start"],
		},
		{
			name: "pipeline",
			testMatch: /pipeline\.spec\.ts$/,
			workers: 1,
			dependencies: ["cold-start"],
		},
	],
});
