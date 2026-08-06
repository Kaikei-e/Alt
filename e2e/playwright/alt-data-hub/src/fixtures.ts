import { test as base } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { testToken } from "../../_shared/ids.js";
import { env } from "./env.js";

/**
 * Suite-wide fixtures.
 *
 * There is no JWT anywhere in this suite. The client certificate *is* the
 * caller's identity — that is the whole reason DataHubService had to leave a
 * listener the public NIC can reach — so the credential is a fixture property
 * of the HTTP client rather than a header on the request.
 *
 * Every client is worker-scoped. Building an `APIRequestContext` with client
 * certificates makes Playwright stand up an internal TLS-terminating proxy,
 * which is far too expensive to pay per test, and there is nothing to isolate:
 * the contexts hold no per-test state. Isolation comes from *naming* — see
 * `token` below — exactly as `_shared/ids.ts` prescribes.
 */

type WorkerFixtures = {
	/**
	 * The data plane, as an allowed peer.
	 *
	 * `ignoreHTTPSErrors` is on because the CA is minted per run and Node has
	 * no way to be told about it from inside an `APIRequestContext`. That is a
	 * deliberate, bounded give-up: it disables verification of the *server's*
	 * leaf, and every assertion these specs make is about who the **server**
	 * admits. The half it gives up is recovered in tests/mtls-boundary.spec.ts,
	 * where `src/mtls-probe.ts` speaks TLS directly with `ca` supplied and
	 * verification on.
	 */
	dataHub: APIRequestContext;

	/**
	 * The same listener with no client certificate at all.
	 *
	 * `ignoreHTTPSErrors` matters more here than anywhere else, and for a
	 * reason worth stating: without it the handshake would fail with
	 * "self-signed certificate in chain" — a client-side complaint about the
	 * throwaway CA that would satisfy `expectTlsHandshakeRejected` while
	 * proving nothing whatsoever about client authentication. With it, the
	 * only thing left that can fail the handshake is alt-data-hub's
	 * `tls.RequireAndVerifyClientCert`, which is the fact under test.
	 */
	anonTLS: APIRequestContext;

	/** The plaintext operator listener: /health + /metrics. */
	ops: APIRequestContext;

	/**
	 * A context with no baseURL, for probing addresses that must not answer.
	 * `_shared/net.ts` takes absolute URLs, so this deliberately has none.
	 */
	prober: APIRequestContext;
};

type TestFixtures = {
	/**
	 * A name unique to (dispatch, worker, test).
	 *
	 * `fullyParallel` distributes individual tests across workers and shards,
	 * so any URL a test asks alt-data-hub about has to be one no sibling is
	 * asking about at the same moment. The Hurl suite used `{{run_id}}`, which
	 * is per *dispatch* — fine when `--jobs 1` guaranteed one request at a
	 * time, not fine now.
	 */
	token: string;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
	dataHub: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.dataHubURL,
				ignoreHTTPSErrors: true,
				clientCertificates: [
					{
						origin: env.dataHubOrigin,
						certPath: env.allowedCertPath,
						keyPath: env.allowedKeyPath,
					},
				],
				extraHTTPHeaders: { "Content-Type": "application/json" },
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	anonTLS: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.dataHubURL,
				ignoreHTTPSErrors: true,
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	ops: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.opsURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	prober: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ ignoreHTTPSErrors: true });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	token: async ({}, use, testInfo) => {
		await use(testToken(testInfo.workerIndex, testInfo.title));
	},
});

export { expect } from "@playwright/test";

/**
 * A URL shaped like the ones pre-processor sends, on a host that resolves
 * nowhere.
 *
 * `.invalid` is reserved by RFC 2606, so nothing in this suite can accidentally
 * cause an outbound fetch — and alt-data-hub never fetches a URL anyway; it
 * stores and looks them up. Embedding the per-test token is what keeps two
 * workers' `CheckArticleExists` probes from being the same question.
 */
export function probeArticleURL(token: string): string {
	return `https://stub.invalid/alt-data-hub/e2e/${token}`;
}
