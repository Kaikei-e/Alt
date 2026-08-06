import { readFileSync } from "node:fs";
import { httpBody, waitForReady } from "../../_shared/readiness.js";
import { seedDocuments } from "../src/corpus.js";
import { env, meiliEnv, seedDocsPath, SharedCorpus } from "../src/env.js";

/**
 * Readiness gate + shared-corpus seed — the replacement for
 * `00-seed-meilisearch.hurl` and for the separate, serial `hurl_run … 00-…`
 * invocation the retired `run.sh` wedged between `compose up` and the suite.
 *
 * Both belong here rather than in a spec, for the same reason: `fullyParallel`
 * has no notion of "run this one first". A readiness check that lives inside
 * the suite is order-dependent by construction, and a seed that lives inside
 * the suite races every worker that reads what it writes.
 *
 * Probing here also collapses the failure mode. A stack that never comes up
 * fails **once**, naming the probe that never passed, instead of failing
 * thirty tests with `connect ECONNREFUSED` and leaving the reader to work out
 * which dependency was actually missing.
 *
 * The order below is the dependency chain, and `waitForReady` runs probes
 * serially precisely so the first broken link is the one reported:
 *
 *   Meilisearch available → search-indexer has bootstrapped its index →
 *   Connect listener answers → seed → the seed is visible *through* the SUT
 */

const READY = { timeout: 120_000, interval: 1_000 } as const;

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null;
}

export default async function globalSetup(): Promise<void> {
	const meili = meiliEnv();

	await waitForReady(
		[
			httpBody(
				`${meili.url}/health`,
				(body) => isRecord(body) && body["status"] === "available",
				"Meilisearch reports available",
			),

			/**
			 * search-indexer must be up **before** the seed, not merely
			 * reachable after it.
			 *
			 * `bootstrap.Run` calls `searchEngine.EnsureIndex` — which creates
			 * `articles` with `id` as its primary key and declares
			 * `tags`/`user_id`/`published_at`/`language` filterable — and only
			 * then constructs and binds the HTTP servers. So a :9300 that
			 * answers is proof the index settings have been applied. Seeding
			 * first would push documents into an index Meilisearch auto-created
			 * with no filterable attributes at all, and every `user_id=` and
			 * `published_after=` assertion would fail with something that reads
			 * like a query bug rather than a bootstrap-ordering bug.
			 */
			httpBody(
				`${env.baseURL}/health`,
				(body) => isRecord(body) && body["status"] === "ok",
				"search-indexer REST :9300 is up, so EnsureIndex has completed",
			),

			/**
			 * The Connect listener is a *second* `http.Server` started from its
			 * own goroutine (`bootstrap/app.go`). A :9300 that answers says
			 * nothing about :9301, and the Hurl suite — which never touched the
			 * port — could not have noticed it failing to bind.
			 */
			httpBody(
				`${env.connectURL}/health`,
				(body) => isRecord(body) && body["status"] === "healthy",
				"search-indexer Connect-RPC :9301 is up",
			),
		],
		READY,
	);

	/**
	 * The canonical fixture corpus, unchanged from what
	 * `00-seed-meilisearch.hurl` pushed. It is read-only for the whole run:
	 * the parity assertions inherited from the Hurl suite are statements about
	 * exactly these documents, and nothing in the suite mutates them. Every
	 * test that needs to *write* gets its own per-worker corpus instead
	 * (`src/fixtures.ts`).
	 */
	await seedDocuments({
		meiliURL: meili.url,
		masterKey: meili.masterKey,
		documents: JSON.parse(readFileSync(seedDocsPath(), "utf8")) as unknown,
		label: "the shared fixture corpus",
	});

	await waitForReady(
		[
			/**
			 * The seed asserted through the dependency that consumes it.
			 *
			 * "The documents landed in Meilisearch" and "search-indexer can see
			 * them" are two different facts — the second additionally requires
			 * the driver's `searchIndex` client, its API key and the index name
			 * to all be right — and the Hurl pre-step only ever established the
			 * first.
			 *
			 * This probe also has to come *last*, and that is not cosmetic:
			 * `driver/meilisearch_cache.go` caches empty result sets under
			 * `{query, limit}` for five minutes. A `q=rust` issued before the
			 * seed had drained would poison the cache for the rest of the run
			 * and no amount of retrying inside a spec would recover it.
			 */
			{
				label: `search-indexer answers "${SharedCorpus.rustQuery}" from the seeded corpus`,
				run: async (api) => {
					const url =
						`${env.baseURL}/v1/search?q=${encodeURIComponent(SharedCorpus.rustQuery)}` +
						`&limit=5`;
					const response = await api.get(url, { timeout: 10_000 });
					if (!response.ok()) {
						throw new Error(`status ${response.status()}`);
					}
					const body: unknown = await response.json();
					const hits = isRecord(body) ? body["hits"] : undefined;
					if (!Array.isArray(hits) || hits.length < SharedCorpus.rustHitCount) {
						throw new Error(
							`expected at least ${SharedCorpus.rustHitCount} hits, got ` +
								`${JSON.stringify(body).slice(0, 300)}`,
						);
					}
				},
			},
		],
		READY,
	);
}
