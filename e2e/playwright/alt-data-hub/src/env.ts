/**
 * Suite configuration, read once per process (the config file, global setup
 * and every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * that failure mode wearing a test-harness hat — a suite pointed at a host
 * that does not exist reports "connection refused" on the first scenario
 * instead of "you forgot to export DATA_HUB_URL", and a suite pointed at the
 * *wrong* host reports green. `run.sh` is the single place these are set.
 *
 * The certificate paths are derived rather than passed: `_lib/suite.sh`'s
 * `suite_pki` writes `<name>.pem` / `<name>-key.pem` per peer and `ca.pem`
 * for the root (e2e/_lib/mint-staging-pki.sh), so deriving them from PKI_DIR
 * plus the peer name keeps one fact — "which identity am I" — in one place.
 * Passing six paths would let the allowlist name and the leaf name drift,
 * and a drift there fails in the TLS handshake with nothing to point at.
 */
import { existsSync } from "node:fs";
import { requiredEnv, runId } from "../../_shared/env.js";

const pkiDir = requiredEnv("PKI_DIR");
const allowedPeer = requiredEnv("ALLOWED_PEER");
const deniedPeer = requiredEnv("DENIED_PEER");

/**
 * Every mTLS client identity whose allowlisted procedures this suite exercises.
 * Kept in lockstep with `suite_pki` in `e2e/playwright/alt-data-hub/run.sh`
 * and `defaultPeerProcedures` in `alt-backend/app/middleware/require_peer_procedure.go`.
 */
export const SUITE_PEERS = [
	"alt-backend",
	"alt-harvester",
	"alt-notifier",
	"pre-processor",
	"search-indexer",
	"tag-generator",
	"recap-worker",
	"rag-orchestrator",
] as const;

export type SuitePeer = (typeof SUITE_PEERS)[number];

export function peerLeafPaths(peer: string): { certPath: string; keyPath: string } {
	return {
		certPath: `${pkiDir}/${peer}.pem`,
		keyPath: `${pkiDir}/${peer}-key.pem`,
	};
}

export function requireLeafFiles(peer: string, certPath: string, keyPath: string): void {
	const missing = [certPath, keyPath].filter((filePath) => !existsSync(filePath));
	if (missing.length > 0) {
		throw new Error(
			`Missing mTLS client leaf material for peer "${peer}": ${missing.join(", ")}. ` +
				`Ensure suite_pki in e2e/playwright/alt-data-hub/run.sh mints "${peer}" under PKI_DIR (${pkiDir}) ` +
				`and that "${peer}" is included in alt-data-hub's DATAHUB_ALLOWED_PEERS when it is an allowed caller.`,
		);
	}
}

export function requireSuitePKI(): void {
	if (!existsSync(`${pkiDir}/ca.pem`)) {
		throw new Error(
			`Missing mTLS CA bundle "${pkiDir}/ca.pem". Run suite_pki in e2e/playwright/alt-data-hub/run.sh before starting Playwright.`,
		);
	}
	const requiredPeers = new Set<string>([...SUITE_PEERS, allowedPeer, deniedPeer]);
	for (const peer of requiredPeers) {
		const { certPath, keyPath } = peerLeafPaths(peer);
		requireLeafFiles(peer, certPath, keyPath);
	}
}

const dataHubURL = requiredEnv("DATA_HUB_URL");
const dataHub = new URL(dataHubURL);

if (dataHub.protocol !== "https:") {
	throw new Error(
		`DATA_HUB_URL must be https — alt-data-hub has no plaintext data-plane ` +
			`listener at all (cmd/datahub/main.go opens :9443 mTLS and :9110 ops). ` +
			`Got: ${dataHubURL}`,
	);
}
if (dataHub.port === "") {
	throw new Error(
		`DATA_HUB_URL must carry an explicit port; the mTLS listener is on :9443 ` +
			`(DATAHUB_LISTEN_ADDR). Got: ${dataHubURL}`,
	);
}

export const env = {
	/** The mutual-TLS data-plane listener. The only externally reachable one. */
	dataHubURL,
	/**
	 * Exact origin, for `clientCertificates[].origin` — Playwright matches it
	 * literally, so `https://alt-data-hub:9443` and `https://alt-data-hub` are
	 * different keys and a mismatch silently sends no certificate at all.
	 */
	dataHubOrigin: dataHub.origin,
	dataHubHost: dataHub.hostname,
	dataHubPort: Number.parseInt(dataHub.port, 10),

	/** The plaintext operator listener: /health + /metrics, nothing else. */
	opsURL: requiredEnv("OPS_URL"),

	/** Ports cmd/datahub must not bind. See tests/topology.spec.ts. */
	absentRestURL: requiredEnv("ABSENT_REST_URL"),
	absentConnectURL: requiredEnv("ABSENT_CONNECT_URL"),
	absentOperatorURL: requiredEnv("ABSENT_OPERATOR_URL"),

	/** On DATAHUB_ALLOWED_PEERS. This suite's positive identity. */
	allowedPeer,
	/** Deliberately *not* on it: a valid chain with the wrong name. */
	deniedPeer,

	/** The throwaway root both ends trust for this run. */
	caPath: `${pkiDir}/ca.pem`,
	allowedCertPath: `${pkiDir}/${allowedPeer}.pem`,
	allowedKeyPath: `${pkiDir}/${allowedPeer}-key.pem`,
	deniedCertPath: `${pkiDir}/${deniedPeer}.pem`,
	deniedKeyPath: `${pkiDir}/${deniedPeer}-key.pem`,

	/** Unique per dispatch; embedded in the URLs each test probes with. */
	runId: runId(),
} as const;

export { ZERO_UUID } from "../../_shared/env.js";
