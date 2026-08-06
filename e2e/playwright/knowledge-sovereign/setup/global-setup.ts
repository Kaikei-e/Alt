import { waitForReady, httpBody } from "../../_shared/readiness.js";
import type { Probe } from "../../_shared/readiness.js";
import { env, procedure } from "../src/env.js";

/**
 * Readiness gate — the direct replacement for `00-setup.hurl`.
 *
 * `run.sh` already waits on the compose healthcheck, but that healthcheck
 * probes **:9501 only** (compose.staging.yaml: `wget --spider
 * http://127.0.0.1:9501/health`). The RPC listener on :9500 is a second
 * `http.Server` started from its own goroutine in `main.go`, so a container
 * compose calls healthy can still be a container whose Connect mux has not
 * bound. That gap is exactly what the retired `00-setup.hurl` retried through.
 *
 * The third probe goes further than the Hurl one did: it calls a procedure
 * that has to reach Postgres. `/health` on either port is
 * `handler.HealthHandler`, an unconditional 200 with no dependency check at
 * all — it stays green with the database on fire. Asserting the seeded
 * projection version instead proves the pool is up *and* that the Atlas
 * migrator actually ran, which is the state every spec in this suite assumes.
 *
 * Probing here rather than inside a spec means a stack that never comes up
 * fails once, with one legible message naming the probe that never passed,
 * instead of failing every test with a connection error. It is also why
 * `00-setup.hurl` must not be ported as a test: a readiness check that lives
 * in the suite is order-dependent by construction, and `fullyParallel` has no
 * notion of "run this one first".
 */

function isHealthy(body: unknown): boolean {
	if (typeof body !== "object" || body === null) return false;
	const record = body as Record<string, unknown>;
	return record["status"] === "ok" && record["service"] === "knowledge-sovereign";
}

/**
 * `GetActiveProjectionVersion` must answer with the row migration
 * `00001_initial_schema.sql` seeds (`version 1`, `status 'active'`).
 *
 * Anything else means the migrator has not finished, so keep polling: a
 * 500 here would otherwise fail the whole suite while the stack was simply
 * still coming up.
 */
const projectionVersionSeeded: Probe = {
	label: `Connect ${env.baseURL}${procedure("GetActiveProjectionVersion")} returns the seeded active version`,
	run: async (api) => {
		const response = await api.post(`${env.baseURL}${procedure("GetActiveProjectionVersion")}`, {
			headers: { "Content-Type": "application/json", "Connect-Protocol-Version": "1" },
			data: {},
			timeout: 10_000,
		});
		if (!response.ok()) {
			throw new Error(`status ${response.status()}: ${(await response.text()).slice(0, 300)}`);
		}
		const body: unknown = await response.json();
		const version =
			typeof body === "object" && body !== null
				? (body as Record<string, unknown>)["version"]
				: undefined;
		const status =
			typeof version === "object" && version !== null
				? (version as Record<string, unknown>)["status"]
				: undefined;
		if (status !== "active") {
			throw new Error(`no active projection version yet: ${JSON.stringify(body).slice(0, 300)}`);
		}
	},
};

export default async function globalSetup(): Promise<void> {
	await waitForReady([
		// Serial and in dependency order: the operator port binds first in
		// main.go, then the RPC port, then the pool is provably usable. A
		// parallel run would report the last link's failure while the first is
		// still the cause.
		httpBody(`${env.metricsURL}/health`, isHealthy, `operator ${env.metricsURL}/health`),
		httpBody(`${env.baseURL}/health`, isHealthy, `RPC ${env.baseURL}/health`),
		projectionVersionSeeded,
	]);
}
