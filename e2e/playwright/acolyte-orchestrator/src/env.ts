/**
 * Suite configuration, read once per process (the config file, global setup and
 * every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately: CLAUDE.md
 * rule 9 forbids warn-and-limp startup config, and a default URL is that same
 * failure mode wearing a test-harness hat — a suite pointed at a host that does
 * not exist reports "connection refused" on scenario 1 instead of "you forgot
 * to export BASE_URL", and a suite pointed at the *wrong* host reports green.
 * `run.sh` is the single place these are set.
 */
import { requiredEnv, requiredSecretFile, runId } from "../../_shared/env.js";

export const env = {
	/** The one listener acolyte-orchestrator binds: `GET /health` + the Connect mux. */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * The port pki-agent-acolyte-orchestrator serves mutual TLS on in
	 * production, from inside this container's network namespace
	 * (`compose/pki.yaml:369-400`, `network_mode: "service:acolyte-orchestrator"`).
	 * The staging slice runs no pki-agent, so nothing may answer here — see
	 * tests/topology.spec.ts.
	 */
	mtlsSidecarURL: requiredEnv("MTLS_SIDECAR_URL"),

	/** Unique per dispatch; keeps seeded report titles apart across reruns. */
	runId: runId(),
} as const;

/**
 * Values only `setup/global-setup.ts` needs.
 *
 * A function rather than a const because `requiredSecretFile` touches the
 * filesystem: making it module-level would mean every one of the four workers
 * re-reads the Meilisearch master key at import time, and a permissions problem
 * on that file would fail the whole suite with a confusing error rather than
 * failing the seed step that actually depends on it.
 */
export function seedEnv() {
	return {
		meiliURL: requiredEnv("MEILI_URL"),
		meiliMasterKey: requiredSecretFile("MEILI_MASTER_KEY_FILE"),
		meiliSeedDocs: requiredEnv("MEILI_SEED_DOCS"),
		searchIndexerURL: requiredEnv("SEARCH_INDEXER_URL"),
		ollamaStubURL: requiredEnv("OLLAMA_STUB_URL"),
	} as const;
}

export { ZERO_UUID } from "../../_shared/env.js";
