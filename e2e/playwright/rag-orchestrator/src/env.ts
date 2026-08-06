/**
 * Suite configuration, read once per process (the config file, the global
 * setup, and every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * exactly that failure mode wearing a test-harness hat — a suite pointed at a
 * host that does not exist reports "connection refused" on the first scenario
 * instead of "you forgot to export CONNECT_URL", and a suite pointed at the
 * *wrong* host reports green. `run.sh` is the single place these are set.
 */
import { requiredEnv, runId } from "../../_shared/env.js";

export const env = {
	/** Echo REST listener (`PORT=9010`): /healthz, /readyz, /metrics, /v1/rag/*. */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * Connect-RPC listener (`CONNECT_PORT=9011`).
	 *
	 * Plaintext h2c in this slice: compose.staging.yaml sets
	 * `PEER_IDENTITY_MODE=disabled`, which is the explicit opt-out branch of
	 * cmd/server/main.go's peer-identity switch and keeps
	 * `connect.CreateConnectServer` (h2c) instead of
	 * `CreateMTLSConnectServer`. The consequence — X-Alt-User-Id is trusted
	 * from any caller that can reach the port — is asserted rather than
	 * assumed in tests/augur-authz.spec.ts.
	 */
	connectURL: requiredEnv("CONNECT_URL"),

	/**
	 * The port rag-orchestrator talks to alt-data-hub *on*, never serves on.
	 *
	 * `DATAHUB_MTLS_URL=https://alt-data-hub:9443` makes this service an mTLS
	 * **client** of the data plane (ADR-000954 D7: there is no plaintext route
	 * to alt_db to degrade onto, which is why run.sh still mints a client leaf
	 * even though no scenario reaches alt-data-hub). cmd/server/main.go opens
	 * exactly two listeners; tests/topology.spec.ts asserts this third one is
	 * closed so an accidentally-added sidecar cannot appear unnoticed.
	 */
	mtlsSidecarURL: requiredEnv("MTLS_SIDECAR_URL"),

	/** Unique per dispatch; only used to keep report/artifact paths apart. */
	runId: runId(),
} as const;
