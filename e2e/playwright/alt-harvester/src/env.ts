/**
 * Suite configuration, read once per process (config file, global setup and
 * every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * exactly that failure mode wearing a test-harness hat. It bites harder in
 * this suite than in any other, because almost every assertion here is a
 * negative: a suite pointed at a host that does not exist would find every
 * surface absent and every port closed, and report green while testing
 * nothing. `run.sh` is the single place these are set.
 */
import { requiredEnv, runId } from "../../_shared/env.js";

export const env = {
	/**
	 * The shared operator listener — the only socket cmd/harvester opens.
	 *
	 * `OPS_LISTEN=:9110` in compose.staging.yaml, `/health` + `/metrics` from
	 * `bootstrap.NewOpsHandler`. Note the unversioned path: this is not a
	 * product surface and deliberately does not live under `/v1`.
	 */
	opsURL: requiredEnv("OPS_URL"),

	/**
	 * The harvester's DNS name on the staging network, as a bare host.
	 *
	 * tests/topology.spec.ts needs to build a URL per *port that must have
	 * nothing bound to it*, so it needs the host separated from the one port
	 * that does answer.
	 */
	harvesterHost: requiredEnv("HARVESTER_HOST"),

	/** Unique per dispatch; only used to keep report/artifact paths apart. */
	runId: runId(),
} as const;

/** A URL on the harvester's host at an arbitrary port. */
export function harvesterURL(port: number, path = "/health"): string {
	return `http://${env.harvesterHost}:${port}${path}`;
}
