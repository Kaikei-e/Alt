import { z } from "zod";

/**
 * Response envelopes, as Zod schemas.
 *
 * The Hurl suite this replaces asserted one JSONPath at a time
 * (`jsonpath "$.hits[0].id" exists`), which says nothing about the fields it
 * did not name and nothing at all about hits 1..n. A schema asserts the whole
 * envelope in one step, so a handler that starts dropping `score` — or that
 * starts returning it as a string — fails here instead of quietly satisfying
 * a spot check on element zero.
 *
 * Object schemas are `passthrough()` by convention: a contract on the fields
 * search-indexer promises, not a freeze on the ones it may add. The two
 * exceptions are the health envelopes, which are `strict()`, because both are
 * a literal string written by a three-line handler — an extra key there is a
 * change to that literal and worth failing on.
 */

export { timestampSchema } from "../../_shared/schemas.js";
export { connectErrorSchema } from "../../_shared/connect.js";
import { nonEmptyArray, timestampSchema } from "../../_shared/schemas.js";

// ---------------------------------------------------------------------------
// REST :9300
// ---------------------------------------------------------------------------

/**
 * `GET /health` on the REST listener.
 *
 * `bootstrap/servers.go` writes the byte string `{"status":"ok"}` — not a
 * struct, not a computed envelope. `.strict()` is therefore exact rather than
 * brittle, and it is what stops this from silently becoming the *Connect*
 * listener's `{"status":"healthy","service":"connect-rpc"}` if the two muxes
 * are ever merged.
 */
export const restHealthSchema = z.object({ status: z.literal("ok") }).strict();

/** `GET /health` on the Connect listener — a deliberately different literal. */
export const connectHealthSchema = z
	.object({ status: z.literal("healthy"), service: z.literal("connect-rpc") })
	.strict();

/**
 * One hit of `GET /v1/search` (`rest.SearchArticlesHit`).
 *
 * `id`, `title`, `content`, `tags` and `score` have no `omitempty`, so they
 * are always on the wire; `language` and `published_at` do, so they are
 * absent — never null — when the indexed document carried neither.
 *
 * `tags` is `z.array` and not `.nullable()` on purpose: the handler explicitly
 * substitutes `[]string{}` for a nil slice, and a `null` here would be a
 * regression the SPA's `hit.tags.map(...)` would crash on.
 */
export const searchHitSchema = z
	.object({
		id: z.string().min(1),
		title: z.string(),
		content: z.string(),
		tags: z.array(z.string()),
		/**
		 * Meilisearch's `_rankingScore`, surfaced because
		 * `newBaseSearchRequest` sets `ShowRankingScore: true`. Bounded 0..1 by
		 * the engine. Asserting the *range* rather than `isNumber` is what
		 * catches the wiring regression the Hurl assertion could not: drop
		 * `ShowRankingScore` and `getFloat64` falls back to `0.0`, which is
		 * still a number.
		 */
		score: z.number().min(0).max(1),
		language: z.string().optional(),
		published_at: timestampSchema.optional(),
	})
	.passthrough();

/**
 * The `GET /v1/search` envelope (`rest.SearchArticlesResponse`).
 *
 * `total` is `len(hits)`, *not* Meilisearch's `estimatedTotalHits` — the REST
 * path has no access to the latter. That is a contract clients get wrong, so
 * `searchEnvelopeIsSelfConsistent` below pins it on every response this suite
 * parses rather than in one dedicated test.
 */
export const searchResponseSchema = z
	.object({
		query: z.string(),
		hits: z.array(searchHitSchema),
		total: z.number().int().nonnegative(),
	})
	.passthrough()
	.refine((body) => body.total === body.hits.length, {
		message:
			"total must equal hits.length — the REST handler sets Total: len(docs) " +
			"(rest/handler.go). A divergence means it started reporting the engine's " +
			"estimatedTotalHits, which is a breaking change for every REST caller.",
	});

export type SearchResponse = z.infer<typeof searchResponseSchema>;

/**
 * The same envelope, for scenarios that seeded the data they read back.
 *
 * Spelled out rather than intersected with the schema above: `z.array(x)`
 * accepts `[]`, which is the single most common way an assertion about a
 * collection stops testing anything — a search path that silently stopped
 * reaching Meilisearch returns `{"query":"…","hits":[],"total":0}` forever and
 * every "is an array" check keeps passing.
 */
export const nonEmptySearchResponseSchema = z
	.object({
		query: z.string().min(1),
		hits: nonEmptyArray(searchHitSchema),
		total: z.number().int().positive(),
	})
	.passthrough()
	.refine((body) => body.total === body.hits.length, {
		message: "total must equal hits.length (rest/handler.go sets Total: len(docs))",
	});

// ---------------------------------------------------------------------------
// Connect-RPC :9301
// ---------------------------------------------------------------------------

/**
 * A proto `int64` as protojson encodes it: a **string**, not a number.
 *
 * connect-go's default JSON codec is plain `protojson`, with no
 * `EmitUnpopulated` and no numeric int64 escape hatch, so
 * `estimated_total_hits` arrives as `"5"`. A suite that asserted
 * `z.number()` here would fail against a perfectly correct service; one that
 * asserted `z.unknown()` would not notice the day it changes.
 */
export const int64Schema = z.union([z.string().regex(/^-?\d+$/), z.number().int()]);

/** Narrows the two int64 wire forms to one number for arithmetic assertions. */
export function asInt64(value: string | number): number {
	return typeof value === "number" ? value : Number.parseInt(value, 10);
}

/**
 * `services.search.v2.SearchHit`.
 *
 * Every field is optional because protojson omits zero values and connect-go
 * installs no `EmitUnpopulated` codec — an empty `tags` list simply is not on
 * the wire. `id` is the exception: the handler copies it straight from the
 * Meilisearch document id, which is the index's primary key and cannot be
 * empty.
 */
export const connectSearchHitSchema = z
	.object({
		id: z.string().min(1),
		title: z.string().optional(),
		content: z.string().optional(),
		tags: z.array(z.string()).optional(),
	})
	.passthrough();

/**
 * `SearchArticlesResponse` for a query that matched nothing.
 *
 * `query` is still present: the handler echoes the caller's (non-empty,
 * already validated) query, so protojson keeps it. `hits` and
 * `estimatedTotalHits` are both absent, which is exactly the shape a
 * connect-es client sees as `{hits: [], estimatedTotalHits: 0n}`.
 */
export const connectEmptySearchResponseSchema = z
	.object({
		query: z.string().min(1),
		hits: z.array(connectSearchHitSchema).optional(),
		estimatedTotalHits: int64Schema.optional(),
	})
	.passthrough();

/** `SearchArticlesResponse` where the scenario seeded the data it reads back. */
export const connectSearchResponseSchema = z
	.object({
		query: z.string().min(1),
		hits: nonEmptyArray(connectSearchHitSchema),
		estimatedTotalHits: int64Schema,
	})
	.passthrough();

/**
 * `SearchRecapsResponse`.
 *
 * Deliberately lenient, and this is the one schema in the file that is: the
 * `recaps` index is created by `bootstrap`'s `EnsureRecapIndex` but nothing in
 * this slice writes documents to it (`RECAP_WORKER_URL` points at the nginx
 * stub, whose HTML the recap index loop fails to decode), so a correct
 * response really is `{}`. What the test built on it asserts is the *status* —
 * 200 rather than the 501 an unconfigured recap path returns — and that
 * whatever `hits` is, it is a list.
 */
export const connectRecapResponseSchema = z
	.object({
		hits: z
			.array(
				z
					.object({
						id: z.string().min(1),
						jobId: z.string().optional(),
						genre: z.string().optional(),
						summary: z.string().optional(),
						topTerms: z.array(z.string()).optional(),
						bullets: z.array(z.string()).optional(),
						tags: z.array(z.string()).optional(),
					})
					.passthrough(),
			)
			.optional(),
		estimatedTotalHits: int64Schema.optional(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Meilisearch
// ---------------------------------------------------------------------------

/** The `202 Accepted` envelope every write to Meilisearch answers with. */
export const meiliEnqueuedTaskSchema = z
	.object({
		taskUid: z.number().int().nonnegative(),
		indexUid: z.string().optional(),
		status: z.string(),
	})
	.passthrough();

/** `GET /tasks/{uid}`. `status` is the only field the seed cares about. */
export const meiliTaskSchema = z
	.object({
		uid: z.number().int().nonnegative(),
		status: z.enum(["enqueued", "processing", "succeeded", "failed", "canceled"]),
	})
	.passthrough();
