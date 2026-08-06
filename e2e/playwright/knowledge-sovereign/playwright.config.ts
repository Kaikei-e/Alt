import { defineApiSuite } from "../_shared/config.js";

/**
 * knowledge-sovereign API E2E.
 *
 * Two listeners, both plain HTTP inside the staging network:
 *
 *   :9500  Connect-RPC JSON — `services.sovereign.v1.KnowledgeSovereignService`
 *          plus a bare `/health` (main.go: `mainMux`)
 *   :9501  operator surface — `/health`, `/metrics`, `/admin/*`
 *          (main.go: `metricsMux`)
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop
 * lives in `defineApiSuite`; only the facts genuinely local to this service
 * are here.
 */
export default defineApiSuite({
	service: "knowledge-sovereign",

	/**
	 * Sized against the **pgx pool**, which is this service's real ceiling.
	 *
	 * `main.go` builds the pool with `pgxpool.New(ctx, cfg.DatabaseURL)` and
	 * the staging DATABASE_URL carries no `pool_max_conns`, so pgxpool falls
	 * back to `max(4, runtime.NumCPU())` — 4 on a 4-vCPU GitHub runner. Four
	 * in-process background loops already draw on that same pool on their own
	 * cadence (partition maintainer, knowledge_trail_projector and
	 * knowledge_home_projector every 2s in staging, projection_health every
	 * 60s), so the suite must not assume it owns all four slots.
	 *
	 * 4 workers keeps at most one in-flight request each and leaves the
	 * projectors able to make progress — which matters, because several specs
	 * assert *that* progress (tests/projections.spec.ts). Pushing higher would
	 * queue requests behind an ACCESS EXCLUSIVE partition DDL and turn a pool
	 * wait into a spurious 30s test timeout.
	 */
	workers: 4,

	globalSetup: "./setup/global-setup.ts",

	projects: [
		{
			name: "api",
			testIgnore: /admin-.*\.spec\.ts$/,
		},
		/**
		 * The `/admin/*` surface on :9501 is the one part of this service with
		 * genuinely global state: `POST /admin/snapshots/create` exports whole
		 * tables and `GET /admin/snapshots/latest` answers "the newest valid
		 * snapshot in the database", not "the one you just made". Two workers
		 * creating snapshots concurrently would make `latest` a coin flip.
		 *
		 * A single-worker project (`TestProject.workers`, Playwright 1.52+)
		 * rather than a `serial` describe, matching alt-backend's rate-limit
		 * project: a serial describe would still run alongside three other
		 * workers, and here the interference is between snapshot writers, not
		 * within one file.
		 *
		 * Each admin spec still seeds its own event before creating a snapshot
		 * — the handler refuses when `max_event_seq <= 0` — so the project is
		 * order-independent even at workers: 1.
		 */
		{
			name: "admin",
			testMatch: /admin-.*\.spec\.ts$/,
			workers: 1,
		},
	],
});
