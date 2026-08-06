import { readFileSync } from "node:fs";
import { request } from "@playwright/test";
import { connectListening, httpBody, httpOk, waitForReady } from "../../_shared/readiness.js";
import { env, seedEnv } from "../src/env.js";

/**
 * Readiness gate + corpus seed — the replacement for `00-setup.hurl` and for
 * the `hurl_run … search-indexer/00-seed-meilisearch.hurl` pre-step the retired
 * `run.sh` wedged between compose and the suite.
 *
 * Both belong here rather than in a spec, for the same reason: `fullyParallel`
 * has no notion of "run this one first". A readiness check that lives inside
 * the suite is order-dependent by construction, and a seed step that lives
 * inside the suite races every worker that reads what it writes.
 *
 * Probing here also collapses the failure mode. A stack that never comes up
 * fails **once**, naming the probe that never passed, instead of failing forty
 * tests with `connect ECONNREFUSED` and leaving the reader to work out which
 * dependency was actually missing.
 *
 * The order below is the dependency chain, and `waitForReady` runs probes
 * serially precisely so the first broken link is the one reported:
 *
 *   Meilisearch → seed → search-indexer sees the seed → Ollama stub →
 *   acolyte REST /health → acolyte Connect mux
 */

const READY = { timeout: 120_000, interval: 1_000 } as const;

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null;
}

export default async function globalSetup(): Promise<void> {
	const seed = seedEnv();

	await waitForReady(
		[
			httpBody(
				`${seed.meiliURL}/health`,
				(body) => isRecord(body) && body["status"] === "available",
				"Meilisearch reports available",
			),
			/**
			 * search-indexer must be up *before* the seed, not merely reachable
			 * after it: `bootstrap.EnsureIndex` creates the `articles` index and
			 * sets its filterable attributes on process start
			 * (search-indexer/app/bootstrap/app.go). Pushing documents into an
			 * index Meilisearch auto-created instead would give a corpus with no
			 * filterable `tags`/`user_id`, which fails in a way that looks like a
			 * query bug rather than a bootstrap-ordering bug.
			 */
			httpOk(`${seed.searchIndexerURL}/health`, "search-indexer has bootstrapped its index"),
		],
		READY,
	);

	await seedMeilisearch(seed);

	await waitForReady(
		[
			/**
			 * The seed asserted through the dependency that actually consumes it.
			 * acolyte's gatherer node talks to search-indexer's REST API, not to
			 * Meilisearch — so "the documents landed" and "the gatherer can see
			 * them" are two different facts, and the retired Hurl pre-step only
			 * established the first. When the run-lifecycle scenarios came back
			 * with empty evidence, that gap was exactly what the old run.sh's
			 * `dump_diagnostics` had to reconstruct by hand afterwards.
			 */
			{
				label: `search-indexer returns hits for "${SEED_PROBE_QUERY}"`,
				run: async (api) => {
					const url = `${seed.searchIndexerURL}/v1/search?q=${encodeURIComponent(
						SEED_PROBE_QUERY,
					)}&limit=5`;
					const response = await api.get(url, { timeout: 10_000 });
					if (!response.ok()) {
						throw new Error(`status ${response.status()}`);
					}
					const body: unknown = await response.json();
					const hits = isRecord(body) ? body["hits"] : undefined;
					if (!Array.isArray(hits) || hits.length === 0) {
						throw new Error(
							`no hits yet — the gatherer node would get empty evidence: ` +
								`${JSON.stringify(body).slice(0, 300)}`,
						);
					}
				},
			},

			/**
			 * NEWS_CREATOR_URL points here (compose.staging.yaml:823). Acolyte's
			 * OllamaGateway calls `/api/generate` and `/api/chat`; `/api/tags` is
			 * the cheapest proof the stub's FastAPI app has finished importing.
			 */
			httpOk(`${seed.ollamaStubURL}/api/tags`, "news-creator-ollama-stub is serving"),

			/**
			 * Not `httpOk`: a 200 is not the same as ready. This is the exact
			 * body `00-setup.hurl` asserted, and it is what proves the Starlette
			 * application factory finished wiring AcolyteConnectService against
			 * the Atlas-migrated database rather than merely binding the port.
			 */
			httpBody(
				`${env.baseURL}/health`,
				(body) =>
					isRecord(body) &&
					body["status"] === "ok" &&
					body["service"] === "acolyte-orchestrator",
				"acolyte-orchestrator REST /health reports ok",
			),

			/**
			 * The REST route and the Connect mount are separate entries in
			 * `main.py`'s `routes=[...]`. A `/health` that answers while the
			 * mount is missing is a real state — it is what a failed
			 * `AcolyteServiceASGIApplication` construction would look like — so
			 * the mux gets its own probe.
			 */
			connectListening(env.baseURL, "/alt.acolyte.v1.AcolyteService/HealthCheck"),
		],
		READY,
	);
}

/** A phrase the seeded corpus answers to; see e2e/fixtures/search-indexer/seed-docs.json. */
const SEED_PROBE_QUERY = "AI infrastructure";

type Seed = ReturnType<typeof seedEnv>;

/**
 * Pushes the canonical fixture corpus into Meilisearch's `articles` index and
 * blocks on the indexing task.
 *
 * Blocking on the task rather than sleeping is the point: Meilisearch's
 * document ingest is asynchronous, `202 Accepted` only means the task was
 * enqueued, and a suite that started querying at that moment would see a
 * partially-populated index on a slow daemon and a full one on a fast daemon.
 */
async function seedMeilisearch(seed: Seed): Promise<void> {
	const documents: unknown = JSON.parse(readFileSync(seed.meiliSeedDocs, "utf8"));
	const api = await request.newContext({
		extraHTTPHeaders: {
			// The key is read from a file and only ever placed in this header —
			// never in a URL, a log line or a compose slice.
			Authorization: `Bearer ${seed.meiliMasterKey}`,
			"Content-Type": "application/json",
		},
	});

	try {
		const response = await api.post(`${seed.meiliURL}/indexes/articles/documents`, {
			data: documents,
			timeout: 30_000,
		});
		if (response.status() !== 202) {
			throw new Error(
				`seeding the Meilisearch articles index failed: ` +
					`${response.status()} ${(await response.text()).slice(0, 500)}`,
			);
		}
		const enqueued: unknown = await response.json();
		const taskUid = isRecord(enqueued) ? enqueued["taskUid"] : undefined;
		if (typeof taskUid !== "number") {
			throw new Error(`Meilisearch did not return a taskUid: ${JSON.stringify(enqueued)}`);
		}

		await waitForReady(
			[
				{
					label: `Meilisearch indexing task ${taskUid} succeeds`,
					run: async (probeApi) => {
						const task = await probeApi.get(`${seed.meiliURL}/tasks/${taskUid}`, {
							headers: { Authorization: `Bearer ${seed.meiliMasterKey}` },
							timeout: 10_000,
						});
						const body: unknown = await task.json();
						const status = isRecord(body) ? body["status"] : undefined;
						if (status === "failed" || status === "canceled") {
							// Terminal-and-wrong. Keep polling would burn the whole
							// budget on a task that will never change.
							throw new Error(
								`indexing task ${taskUid} ended as "${String(status)}": ` +
									`${JSON.stringify(body).slice(0, 400)}`,
							);
						}
						if (status !== "succeeded") {
							throw new Error(`task ${taskUid} is "${String(status)}"`);
						}
					},
				},
			],
			{ timeout: 60_000, interval: 250 },
		);
	} finally {
		await api.dispose();
	}
}
