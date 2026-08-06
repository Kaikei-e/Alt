/**
 * Suite configuration, read once per process (global setup and every worker
 * each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * exactly that failure mode wearing a test-harness hat — a suite pointed at a
 * host that does not exist reports "connection refused" on the first spec
 * instead of "you forgot to export BASE_URL", and a suite pointed at the
 * *wrong* host reports green. `run.sh` is the single place these are set.
 */
import { requiredEnv, runId } from "../../_shared/env.js";

export const env = {
	/**
	 * Connect-RPC JSON listener (`LISTEN_ADDR`, :9500 in staging). Also serves
	 * a bare `/health` — main.go registers `handler.HealthHandler` on both
	 * muxes, and this is the one the *service* considers its front door.
	 */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * Operator listener (`METRICS_ADDR`, :9501 in staging).
	 *
	 * Named METRICS_URL for continuity with the retired Hurl suite and its CI
	 * job, but it carries three distinct surfaces: `/health` (the port the
	 * compose healthcheck actually probes), `/metrics` (Prometheus), and the
	 * whole `/admin/*` snapshot / retention / storage API.
	 */
	metricsURL: requiredEnv("METRICS_URL"),

	/** Unique per dispatch. Embedded in every dedupe_key this suite writes. */
	runId: runId(),
} as const;

/** The one Connect service this binary mounts (main.go: `mainMux.Handle`). */
export const SOVEREIGN_SERVICE = "services.sovereign.v1.KnowledgeSovereignService";

/** Fully-qualified unary path for a method on that service. */
export function procedure(method: string): string {
	return `/${SOVEREIGN_SERVICE}/${method}`;
}

/**
 * Every `/admin/*` route `metricsMux` registers, with its declared verb.
 *
 * Kept as one table because three specs need it for three different reasons:
 * the topology spec proves none of it leaks onto :9500, the method spec proves
 * Go 1.22's method-aware ServeMux answers 405 rather than 404 on the wrong
 * verb, and the admin specs call it. A route added to
 * handler/{snapshot,retention,storage}_handler.go and not added here is a
 * surface with no topology assertion at all.
 */
export const ADMIN_ROUTES = [
	{ method: "POST", path: "/admin/snapshots/create" },
	{ method: "GET", path: "/admin/snapshots/list" },
	{ method: "GET", path: "/admin/snapshots/latest" },
	{ method: "POST", path: "/admin/retention/run" },
	{ method: "GET", path: "/admin/retention/status" },
	{ method: "GET", path: "/admin/retention/eligible" },
	{ method: "GET", path: "/admin/storage/stats" },
] as const;

export { ZERO_UUID } from "../../_shared/env.js";
