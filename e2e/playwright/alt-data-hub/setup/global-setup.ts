import { request } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { waitForReady, httpOk } from "../../_shared/readiness.js";
import type { Probe } from "../../_shared/readiness.js";
import { env, requireSuitePKI } from "../src/env.js";
import { connectHealthSchema } from "../src/schemas.js";

/**
 * Readiness gate — the direct replacement for `00-setup.hurl`.
 *
 * That scenario had `retry: 60` on `GET {{data_hub_url}}/health` and had to
 * run first, which is why the Hurl runner invoked it as a separate serial
 * pass before the suite. `fullyParallel` has no notion of "run this one
 * first", so a readiness check that lives in a spec is order-dependent by
 * construction; it belongs here.
 *
 * `run.sh` already waits on the compose healthcheck, and that healthcheck is
 * genuinely good — `/app-entry healthcheck` GETs OPS_LISTEN/health *and*
 * dials DATAHUB_LISTEN_ADDR, so it cannot report healthy with a dead mTLS
 * goroutine (ADR-000784). What it still cannot see is anything behind the
 * listener: it dials :9443, it does not complete a handshake, and it never
 * touches the database pool. The probes below walk that dependency chain in
 * order, so a stack that never comes up fails **once**, naming the link that
 * did not close, instead of failing every test with a transport error.
 */

const SVC = "/services.datahub.v1.DataHubService";

/**
 * One read-only database probe per allowed peer identity in `defaultPeerProcedures`,
 * so `globalSetup` probes a procedure `env.allowedPeer` is authorized to call.
 */
const READINESS_PROBE_BY_PEER: Readonly<Record<string, string>> = {
	"alt-backend": `${SVC}/CountBackfillArticles`,
	"pre-processor": `${SVC}/HasUnsummarizedArticles`,
	"search-indexer": `${SVC}/GetLatestArticleTimestamp`,
	"tag-generator": `${SVC}/ListUntaggedArticles`,
	"recap-worker": `${SVC}/ListFeedsInWindow`,
	"rag-orchestrator": `${SVC}/ListRecentArticles`,
	"alt-harvester": `${SVC}/ListScrapingDomains`,
	"alt-notifier": `${SVC}/GetNotificationBacklogAge`,
};

function readinessProbeProcedure(peer: string): string {
	const proc = READINESS_PROBE_BY_PEER[peer];
	if (!proc) {
		throw new Error(
			`No readiness database probe configured for ALLOWED_PEER="${peer}". ` +
				`Supported peers: ${Object.keys(READINESS_PROBE_BY_PEER).join(", ")}`,
		);
	}
	return proc;
}

async function mtlsContext(): Promise<APIRequestContext> {
	return request.newContext({
		baseURL: env.dataHubURL,
		// See src/fixtures.ts for why verification of the server leaf is off
		// here and asserted directly in tests/mtls-boundary.spec.ts instead.
		ignoreHTTPSErrors: true,
		clientCertificates: [
			{
				origin: env.dataHubOrigin,
				certPath: env.allowedCertPath,
				keyPath: env.allowedKeyPath,
			},
		],
	});
}

export default async function globalSetup(): Promise<void> {
	requireSuitePKI();
	const probeProcedure = readinessProbeProcedure(env.allowedPeer);
	const mtls = await mtlsContext();

	try {
		const probes: readonly Probe[] = [
			/**
			 * 1. The process is up at all. Plaintext, no credential, so a failure
			 *    here is unambiguously "the container is not serving yet" rather
			 *    than anything about certificates.
			 */
			httpOk(`${env.opsURL}/health`, `ops listener ${env.opsURL}/health`),

			/**
			 * 2. The mutual-TLS handshake completes and the Connect mux answers.
			 *    This is the link the compose healthcheck's TCP dial cannot see:
			 *    a certificate file mounted late, or an allowlist that does not
			 *    contain this run's peer name, both leave :9443 accepting
			 *    connections and rejecting every one of them.
			 */
			{
				label: `mTLS ${env.dataHubURL}/health as ${env.allowedPeer}`,
				run: async () => {
					const response = await mtls.get("/health", { timeout: 10_000 });
					const body = await response.text();
					if (!response.ok()) {
						throw new Error(`status ${response.status()}: ${body.slice(0, 300)}`);
					}
					if (!connectHealthSchema.safeParse(JSON.parse(body)).success) {
						throw new Error(`unexpected health body: ${body.slice(0, 300)}`);
					}
				},
			},

			/**
			 * 3. A procedure that actually reads the database answers for
			 *    `env.allowedPeer` (`CountBackfillArticles` for `alt-backend`).
			 *    The Atlas migrator is a `service_completed_successfully`
			 *    dependency, so compose orders it — but the pgx pool opens lazily
			 *    (internal/bootstrap/dbboot), and the first acquire is where a
			 *    half-migrated schema shows up.
			 */
			{
				label: `mTLS ${probeProcedure} as ${env.allowedPeer} (database pool)`,
				run: async () => {
					const response = await mtls.post(probeProcedure, {
						headers: { "Content-Type": "application/json" },
						data: {},
						timeout: 10_000,
					});
					if (response.status() !== 200) {
						throw new Error(
							`status ${response.status()}: ${(await response.text()).slice(0, 300)}`,
						);
					}
				},
			},
		];

		await waitForReady(probes);
	} finally {
		await mtls.dispose();
	}
}
