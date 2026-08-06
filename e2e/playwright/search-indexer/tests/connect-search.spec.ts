import { expect, test } from "../src/fixtures.js";
import { callUnary } from "../../_shared/connect.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { Procedure } from "../src/env.js";
import {
	asInt64,
	connectEmptySearchResponseSchema,
	connectSearchResponseSchema,
} from "../src/schemas.js";

/**
 * `SearchService.SearchArticles` behaviour — new coverage, and the only place
 * in the tree where the paginated search path is exercised over the wire.
 *
 * The REST endpoint and this RPC are *not* two views of the same thing. REST
 * reports `total = len(hits)` and has no offset at all; the RPC reports
 * Meilisearch's `estimated_total_hits` and takes an offset, which is why
 * `e2e/hurl/search-indexer/README.md` had to tell clients needing a true total
 * to "call the Connect-RPC path on :9301" — a path it then declined to test.
 *
 * Every scenario reads the corpus its own worker seeded (`src/fixtures.ts`):
 * five documents under one `user_id`, all matching one nonce term. That is
 * what makes exact page and count assertions possible without any test
 * depending on another having run.
 */
test.describe("SearchArticles", () => {
	test("returns the caller's whole corpus", { tag: "@smoke" }, async ({ connect, corpus }) => {
		const body = await expectJsonStatus(
			await callUnary(connect, Procedure.searchArticles, {
				query: corpus.nonce,
				userId: corpus.userId,
				limit: 10,
			}),
			200,
			connectSearchResponseSchema,
		);

		expect(body.query).toBe(corpus.nonce);
		expect(body.hits.map((hit) => hit.id).sort()).toEqual(
			corpus.docs.map((doc) => doc.id).sort(),
		);

		// `estimated_total_hits` is a proto `int64`, so protojson puts it on the
		// wire as the *string* `"5"` — connect-go installs no
		// `EmitUnpopulatedJSONCodec` and no numeric-int64 option. A suite
		// asserting `z.number()` here would fail against a correct service, and
		// one asserting nothing would not notice the day it changes. `asInt64`
		// narrows the two legal forms so the count itself can be asserted.
		expect(asInt64(body.estimatedTotalHits)).toBe(corpus.docs.length);

		// The hit body, not just its id: `SearchHit` carries title, content and
		// tags, and `handler.go` substitutes `[]string{}` for a nil tag slice
		// before marshalling — which protojson then drops entirely, so an
		// absent `tags` is correct and a `null` would not be.
		const first = body.hits.find((hit) => hit.id === corpus.docs[0]?.id);
		expect(first?.title).toContain(corpus.nonce);
		expect(first?.tags).toEqual(["e2e", corpus.nonce]);
	});

	test("an omitted limit falls back to the driver default", { tag: "@contract" }, async ({
		connect,
		corpus,
	}) => {
		// protojson omits a zero `limit`, so the handler receives 0 and passes
		// it straight through. `SearchByUserIDWithPagination` is what turns that
		// into 20 (driver/meilisearch_driver.go). Forward a literal 0 to
		// Meilisearch instead and every caller that omits the field gets zero
		// hits — a total outage with a 200 status, which no health check would
		// catch.
		const body = await expectJsonStatus(
			await callUnary(connect, Procedure.searchArticles, {
				query: corpus.nonce,
				userId: corpus.userId,
			}),
			200,
			connectSearchResponseSchema,
		);
		expect(body.hits).toHaveLength(corpus.docs.length);
	});

	test("offset walks disjoint pages", { tag: "@contract" }, async ({ connect, corpus }) => {
		// Three requests, because a single page proves nothing about paging.
		// The assertions that matter are that the pages are disjoint, that they
		// reassemble into the whole corpus, and that the last one is short —
		// the shape a client's "keep fetching until the page is smaller than
		// the limit" loop depends on.
		const pages = await Promise.all(
			[0, 2, 4].map(async (offset) =>
				expectJsonStatus(
					await callUnary(connect, Procedure.searchArticles, {
						query: corpus.nonce,
						userId: corpus.userId,
						limit: 2,
						offset,
					}),
					200,
					connectSearchResponseSchema,
				),
			),
		);

		expect(pages.map((page) => page.hits.length)).toEqual([2, 2, 1]);

		const seen = pages.flatMap((page) => page.hits.map((hit) => hit.id));
		expect(new Set(seen).size, "pages overlap — offset is not being applied").toBe(seen.length);
		expect(seen.sort()).toEqual(corpus.docs.map((doc) => doc.id).sort());

		// The count is of *matches*, not of the returned page. A handler
		// reporting `len(hits)` here — the REST endpoint's semantics — would
		// answer 2, and every "showing 2 of 2" UI built on it would be wrong.
		for (const page of pages) {
			expect(asInt64(page.estimatedTotalHits)).toBe(corpus.docs.length);
		}
	});

	test("an offset past the end is an empty page, not an error", { tag: "@contract" }, async ({
		connect,
		corpus,
	}) => {
		// `offset < 0` is the only offset the handler rejects, so walking off
		// the end has to be an ordinary empty result. A 500 here would turn the
		// last iteration of every pagination loop into a page of the caller's
		// error handling.
		const body = await expectJsonStatus(
			await callUnary(connect, Procedure.searchArticles, {
				query: corpus.nonce,
				userId: corpus.userId,
				limit: 2,
				offset: 500,
			}),
			200,
			connectEmptySearchResponseSchema,
		);
		expect(body.hits ?? []).toHaveLength(0);
	});

	test("does not return another tenant's documents", { tag: "@authz" }, async ({
		connect,
		corpus,
	}) => {
		// The leakage assertion in the direction that matters: a query that
		// definitely has matches in the index, issued as a user who owns none
		// of them. `BuildUserFilter` is the only thing standing between the two,
		// and it is applied inside the driver — below the handler, below the
		// usecase — so nothing above it can be unit-tested into proving this.
		const body = await expectJsonStatus(
			await callUnary(connect, Procedure.searchArticles, {
				query: corpus.nonce,
				userId: corpus.foreignUserId,
				limit: 10,
			}),
			200,
			connectEmptySearchResponseSchema,
		);
		expect(body.hits ?? []).toHaveLength(0);
		expect(asInt64(body.estimatedTotalHits ?? 0)).toBe(0);
	});

	test("does not return the shared corpus to a corpus owner", { tag: "@authz" }, async ({
		connect,
		corpus,
	}) => {
		// The mirror of the above, and the reason both exist: the first shows a
		// user seeing none of *someone else's* documents, this shows a user
		// seeing none of the documents that match a *different query they do
		// not own*. `rust` matches two documents owned by `alice` and `bob`; a
		// filter dropped anywhere on the path returns them here.
		const body = await expectJsonStatus(
			await callUnary(connect, Procedure.searchArticles, {
				query: "rust",
				userId: corpus.userId,
				limit: 10,
			}),
			200,
			connectEmptySearchResponseSchema,
		);
		expect(body.hits ?? []).toHaveLength(0);
	});

	test("the caching layer does not cross tenants", { tag: "@authz" }, async ({
		connect,
		corpus,
	}) => {
		// `driver/meilisearch_cache.go` puts `user_id` in the cache key
		// specifically so a hit can only ever be served to the caller that
		// produced it, and hashes the whole key so a user id containing the
		// delimiter cannot collide with another. Issuing the *same query* under
		// two identities back to back is what would surface a key that had
		// dropped the user: the second call would be served the first's five
		// documents out of the LRU without Meilisearch being consulted at all.
		const owner = await expectJsonStatus(
			await callUnary(connect, Procedure.searchArticles, {
				query: corpus.nonce,
				userId: corpus.userId,
				limit: 3,
			}),
			200,
			connectSearchResponseSchema,
		);
		expect(owner.hits.length).toBe(3);

		const intruder = await expectJsonStatus(
			await callUnary(connect, Procedure.searchArticles, {
				query: corpus.nonce,
				userId: corpus.foreignUserId,
				limit: 3,
			}),
			200,
			connectEmptySearchResponseSchema,
		);
		expect(intruder.hits ?? []).toHaveLength(0);
	});
});
