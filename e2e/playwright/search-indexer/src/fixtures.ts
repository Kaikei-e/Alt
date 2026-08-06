import { test as base } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { buildWorkerDocs, corpusNonce, seedDocuments } from "./corpus.js";
import type { WorkerCorpus } from "./corpus.js";
import { env, meiliEnv } from "./env.js";

/**
 * Suite-wide fixtures.
 *
 * search-indexer's plaintext listeners authenticate nobody — `newHTTPServer`
 * wraps `/v1/search` in a rate limiter and nothing else, and the Connect mux
 * carries only the rate-limit and OTel interceptors — so there is no session
 * to establish and the HTTP clients are cheap, worker-scoped context objects.
 *
 * What each worker does need of its own is a **corpus**, and that is the
 * fixture that replaces the Hurl suite's serial pre-step. The old runner ran
 * `00-seed-meilisearch.hurl` on its own, before the suite, precisely because
 * every later file read what it wrote; here the shared half of that corpus is
 * seeded once in `setup/global-setup.ts` and the mutable half is per-worker,
 * so no spec depends on another having run.
 *
 * See `src/corpus.ts` for why the seed blocks on the Meilisearch task rather
 * than letting the specs poll: search-indexer caches empty result sets for
 * five minutes, which makes "query until it appears" actively wrong here.
 */

export type WorkerFixtures = {
	/** REST :9300 — `/health` and `/v1/search`. */
	rest: APIRequestContext;
	/** Connect-RPC :9301, JSON codec. */
	connect: APIRequestContext;
	/**
	 * Connect-RPC :9301 with **no** default headers.
	 *
	 * Playwright's `extraHTTPHeaders` cannot be removed per request, so proving
	 * "connect-go rejects an unsupported Content-Type" or "the
	 * Connect-Protocol-Version header is not required" needs a context that
	 * never set them in the first place.
	 */
	connectBare: APIRequestContext;
	/** Meilisearch, authenticated with the staging master key. */
	meili: APIRequestContext;
	/**
	 * No base URL and no headers.
	 *
	 * Needed for three things Playwright cannot express on a configured
	 * context: probing a port that must refuse the connection, addressing a
	 * *different* listener than the one a spec is otherwise talking to, and
	 * proving Meilisearch rejects a caller that sends no `Authorization` at
	 * all (`extraHTTPHeaders` cannot be removed per request).
	 */
	bare: APIRequestContext;
	/** Five documents in the shared `articles` index that only this worker can see. */
	corpus: WorkerCorpus;
};

export const test = base.extend<Record<never, never>, WorkerFixtures>({
	rest: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.baseURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	connect: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.connectURL,
				extraHTTPHeaders: { "Content-Type": "application/json" },
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	connectBare: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.connectURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	meili: [
		async ({ playwright }, use) => {
			const meili = meiliEnv();
			const context = await playwright.request.newContext({
				baseURL: meili.url,
				// The key is read from a file and only ever placed in this header —
				// never in a URL, a log line, or a rendered compose slice.
				extraHTTPHeaders: { Authorization: `Bearer ${meili.masterKey}` },
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	bare: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext();
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	corpus: [
		async ({}, use, workerInfo) => {
			const nonce = corpusNonce(workerInfo.workerIndex);
			// Two distinct derived ids so a `user_id` can never be matched as a
			// *search term* by accident: the tenant negative below would otherwise
			// be satisfied by full-text recall rather than by the filter.
			const userId = `usr-${nonce}`;
			const foreignUserId = `nul-${nonce}`;
			const docs = buildWorkerDocs(nonce, userId);
			const meili = meiliEnv();

			await seedDocuments({
				meiliURL: meili.url,
				masterKey: meili.masterKey,
				documents: docs,
				label: `worker ${workerInfo.workerIndex}'s corpus (${nonce})`,
			});

			// No teardown, deliberately. The slice is destroyed with
			// `docker compose down -v` per dispatch, so deleting these documents
			// would buy nothing and would race a sibling worker's search — the
			// same reasoning as `_shared/ids.ts`: isolation is by naming.
			await use({
				nonce,
				userId,
				foreignUserId,
				docs,
				firstPublishedAt: docs[0]?.published_at ?? 0,
			});
		},
		{ scope: "worker" },
	],
});

export { expect } from "@playwright/test";
