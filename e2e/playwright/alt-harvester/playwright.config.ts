import { defineApiSuite } from "../_shared/config.js";

/**
 * alt-harvester API E2E — the job-runner third of the three-binary split
 * (ADR-000954).
 *
 * This is the fleet's only suite whose service under test has no product
 * surface at all. `cmd/harvester/main.go` builds no Echo instance and no
 * Connect mux; the single socket it opens is the shared operator listener on
 * :9110, serving `/health` for the compose probe and `/metrics` for the
 * `alt-harvester` scrape job. Almost every assertion here is therefore a
 * *negative* — a claim about what this binary does not serve — plus the two
 * positives that keep those negatives meaningful.
 */
export default defineApiSuite({
	service: "alt-harvester",

	/**
	 * Sized against the ops listener, which is the only downstream there is.
	 *
	 * There is no DB pool to exhaust: `di.NewHarvesterComponents` has no
	 * `AltDBRepository` field and `cmd/harvester`'s dependency graph contains
	 * neither `alt_db` nor pgx (ADR-000954 Wave 3; di/import_boundary_test.go).
	 * There is no rate limiter on this listener either — `bootstrap.NewOpsHandler`
	 * installs no middleware whatsoever.
	 *
	 * What the listener does share is a process with five scheduler goroutines,
	 * one of which (outbox-worker) ticks every 5 seconds and holds a 5-minute
	 * timeout. 4 workers keep the suite fast without starving that tick on a
	 * two-core CI runner; going wider would only queue on the one thing that
	 * does serialise, `promhttp`'s registry gather.
	 */
	workers: 4,

	globalSetup: "./setup/global-setup.ts",

	/**
	 * No `workers: 1` project. Every test here is a read-only observation of a
	 * listener — the suite cannot mutate anything, because the binary exposes
	 * nothing that writes. The `--jobs 1` the Hurl runner used was for readable
	 * output, not for correctness (e2e/hurl/alt-harvester/run.sh says so), and
	 * nothing is lost by dropping it.
	 */
});
