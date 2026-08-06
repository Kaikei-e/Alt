import { request } from "@playwright/test";
import { waitForReady } from "../../_shared/readiness.js";
import { workerToken } from "../../_shared/ids.js";
import { meiliEnqueuedTaskSchema, meiliTaskSchema } from "./schemas.js";

/**
 * Corpus seeding — the replacement for `00-seed-meilisearch.hurl`, and the
 * mechanism that makes `fullyParallel` safe against a service with a global
 * result cache.
 *
 * Two facts about search-indexer force the design here, and both are invisible
 * from the outside:
 *
 * 1. **Negative results are cached.** `driver/meilisearch_cache.go` fronts
 *    every search path with a 1024-entry LRU on a 5-minute TTL, keyed by
 *    `{query, user_id, filter, offset, limit}`, and `Search` /
 *    `SearchByUserIDWithPagination` call `cache.put` unconditionally — an
 *    empty result set included. So the usual "write, then poll until it shows
 *    up" shape is a trap here: the first poll caches the miss and the next
 *    five minutes of polling are served from that cache, no matter what
 *    Meilisearch has since indexed. The convergence has to be asserted at the
 *    Meilisearch task boundary *before* the first query reaches
 *    search-indexer, which is what `seedDocuments` does.
 * 2. **Documents are shared.** There is one `articles` index for the whole
 *    slice, so isolation has to come from naming. Each worker seeds under a
 *    nonce nothing else uses and filters on a `user_id` nothing else owns.
 */

/** A document as Meilisearch stores it in the `articles` index. */
export type CorpusDoc = {
	readonly id: string;
	readonly title: string;
	readonly content: string;
	readonly tags: readonly string[];
	readonly user_id: string;
	/**
	 * Unix **seconds**, because `EnsureIndex` declares `published_at`
	 * filterable and `driver/models.go` stores it numerically so
	 * `published_at >= X AND published_at <= Y` works as a Meilisearch filter.
	 * An RFC3339 string here would index fine and silently match no date
	 * window at all.
	 */
	readonly published_at: number;
	readonly language?: string;
};

export type WorkerCorpus = {
	/**
	 * The single search term every document in this worker's corpus carries,
	 * in title and content.
	 *
	 * Pure lowercase letters, no digits and no separators, which is not
	 * cosmetic: Meilisearch tokenises on separators, so a hyphenated token
	 * would become several words and a multi-word query returns documents
	 * matching only *some* of them (the `words` ranking rule drops terms).
	 * Two workers would then see each other's documents. One long letter-only
	 * word is a single token, is nobody's prefix, and is far outside the
	 * 2-edit typo tolerance Meilisearch allows for words of this length.
	 */
	readonly nonce: string;
	/** The `user_id` every document in this corpus belongs to. */
	readonly userId: string;
	/** A syntactically valid `user_id` that owns nothing — the tenant negative. */
	readonly foreignUserId: string;
	readonly docs: readonly CorpusDoc[];
	/** `published_at` of `docs[0]`, in Unix seconds. Later docs are +1 day each. */
	readonly firstPublishedAt: number;
};

const DAY_SECONDS = 86_400;

/**
 * 2026-01-01T00:00:00Z.
 *
 * A fixed instant rather than `Date.now()`: the date-window assertions state
 * exact boundaries, and a corpus that moved with the clock would make a
 * failure impossible to reproduce a day later.
 */
const CORPUS_EPOCH = 1_767_225_600;

/** Five documents: enough for `limit=2` pages at offsets 0, 2 and 4. */
const CORPUS_SIZE = 5;

/**
 * Folds a token into lowercase letters only.
 *
 * `workerToken` mixes digits (the dispatch's `RUN_ID` is a Unix timestamp in
 * CI) and hyphens. Digits risk a tokeniser boundary at the digit/letter
 * transition and hyphens guarantee one, so both are mapped away rather than
 * stripped — stripping would collapse `…w1…` and `…w11…` for two different
 * workers onto the same string.
 */
function lettersOnly(token: string): string {
	return token
		.toLowerCase()
		.replace(/[^a-z0-9]/g, "")
		.replace(/[0-9]/g, (digit) => "qrstuvwxyz"[Number(digit)] ?? "q");
}

/** The per-worker search term. `zsi` prefix so it can never collide with a real word. */
export function corpusNonce(workerIndex: number): string {
	return `zsi${lettersOnly(workerToken(workerIndex))}`;
}

/**
 * Builds this worker's five documents.
 *
 * `docs[0]` carries `language: "en"` and the others carry none, which is what
 * lets tests/search-rest.spec.ts assert both halves of the handler's
 * `json:"language,omitempty"` in one query instead of asserting only the
 * branch that happens to be populated.
 */
export function buildWorkerDocs(nonce: string, userId: string): readonly CorpusDoc[] {
	return Array.from({ length: CORPUS_SIZE }, (_unused, index): CorpusDoc => {
		const base = {
			id: `${nonce}-doc-${index}`,
			title: `${nonce} document ${index}`,
			content:
				`${nonce} is the marker term for search-indexer end-to-end document ` +
				`number ${index}. It exists so a worker can assert on a corpus no ` +
				`other worker can see.`,
			tags: ["e2e", nonce] as const,
			user_id: userId,
			published_at: CORPUS_EPOCH + index * DAY_SECONDS,
		} satisfies CorpusDoc;
		return index === 0 ? { ...base, language: "en" } : base;
	});
}

/** `published_at` of the document at `index`, as the RFC3339 the REST handler emits. */
export function publishedAtRFC3339(index: number): string {
	return `${new Date((CORPUS_EPOCH + index * DAY_SECONDS) * 1_000).toISOString().slice(0, 19)}Z`;
}

/**
 * Pushes documents into Meilisearch's `articles` index and blocks until the
 * indexing task reports `succeeded`.
 *
 * Blocking on the task rather than sleeping is the point: `202 Accepted` only
 * means the task was enqueued, so a suite that started querying at that
 * moment would see a partially-populated index on a slow daemon and a full
 * one on a fast daemon. It is also what keeps the driver's negative-result
 * cache out of the picture — by the time any query reaches search-indexer,
 * there is nothing left to converge on.
 */
export async function seedDocuments(options: {
	readonly meiliURL: string;
	readonly masterKey: string;
	readonly documents: unknown;
	/** Prose for the failure message: whose corpus this is. */
	readonly label: string;
}): Promise<void> {
	const auth = { Authorization: `Bearer ${options.masterKey}` };
	const api = await request.newContext({
		extraHTTPHeaders: { ...auth, "Content-Type": "application/json" },
	});

	let taskUid: number;
	try {
		const response = await api.post(`${options.meiliURL}/indexes/articles/documents`, {
			data: options.documents,
			timeout: 30_000,
		});
		const text = await response.text();
		if (response.status() !== 202) {
			throw new Error(
				`seeding ${options.label} into the Meilisearch articles index failed: ` +
					`${response.status()} ${text.slice(0, 500)}`,
			);
		}
		taskUid = meiliEnqueuedTaskSchema.parse(JSON.parse(text)).taskUid;
	} finally {
		await api.dispose();
	}

	await waitForReady(
		[
			{
				label: `Meilisearch indexing task ${taskUid} (${options.label}) succeeds`,
				run: async (probe) => {
					const response = await probe.get(`${options.meiliURL}/tasks/${taskUid}`, {
						headers: auth,
						timeout: 10_000,
					});
					const task = meiliTaskSchema.parse(JSON.parse(await response.text()));
					if (task.status === "failed" || task.status === "canceled") {
						// Terminal and wrong. Continuing to poll would burn the whole
						// budget on a task whose status can never change again, and
						// report a timeout instead of the schema error that caused it.
						throw new Error(
							`indexing task ${taskUid} ended as "${task.status}" — the corpus ` +
								`is not in the index and every search assertion below would ` +
								`fail for a reason that has nothing to do with the handler.`,
						);
					}
					if (task.status !== "succeeded") {
						throw new Error(`task ${taskUid} is "${task.status}"`);
					}
				},
			},
		],
		{ timeout: 60_000, interval: 250 },
	);
}
