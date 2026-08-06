import { expect, test } from "../src/fixtures.js";
import {
	expectHeader,
	expectJsonStatus,
	expectStatus,
	expectStatusIn,
} from "../../_shared/http.js";
import { publishedAtRFC3339 } from "../src/corpus.js";
import { SharedCorpus } from "../src/env.js";
import { nonEmptySearchResponseSchema, searchResponseSchema } from "../src/schemas.js";

/**
 * `GET /v1/search` on the plaintext REST listener — the port of
 * `02-search-basic`, `03-search-pagination-limit`, `04-search-user-scoped`,
 * `05-search-empty-query` and `06-search-limit-bounds`, with the spot checks
 * replaced by envelope assertions.
 *
 * Two paths hang off this one route (`rest/handler.go`), and which one runs is
 * decided entirely by whether `user_id` is present:
 *
 *   absent   → `SearchArticlesUsecase`, unfiltered, default limit **50**,
 *              the path RAG/BM25 callers use
 *   present  → `SearchByUserUsecase.ExecuteWithPagination`, a Meilisearch
 *              `user_id = "…"` filter, default limit **20**
 *
 * The two differ in more than the filter — different defaults, different
 * validation, different date-parameter support — so every scenario below says
 * which one it is exercising.
 */

/** `?q=…` with the parameters a scenario cares about, in a readable form. */
function searchPath(params: Record<string, string>): string {
	const query = new URLSearchParams(params).toString();
	return `/v1/search?${query}`;
}

test.describe("unfiltered search (SearchArticlesUsecase)", () => {
	test(
		"returns the seeded matches with a complete hit envelope",
		{ tag: "@smoke" },
		async ({ rest }) => {
			// Parity with 02-search-basic.hurl, which asserted `hits count >= 2`
			// and then five `exists`/`isString` probes against `hits[0]` alone.
			// The schema covers *every* hit, so a handler that populated element
			// zero and dropped `tags` on the rest — the shape a partial mapping
			// bug actually takes — fails here.
			const response = await rest.get(
				searchPath({ q: SharedCorpus.rustQuery, limit: "10" }),
			);
			const body = await expectJsonStatus(response, 200, nonEmptySearchResponseSchema);

			expect(body.query).toBe(SharedCorpus.rustQuery);
			expect(body.hits.length).toBeGreaterThanOrEqual(SharedCorpus.rustHitCount);

			// Naming the documents, which Hurl's `count >= 2` did not: two hits
			// could be any two rows. These are the only two documents in
			// e2e/fixtures/search-indexer/seed-docs.json carrying "rust", and
			// they are owned by different users — so this also establishes that
			// the *unfiltered* path really is unfiltered.
			const ids = body.hits.map((hit) => hit.id);
			expect(ids).toContain(SharedCorpus.aliceDocId);
			expect(ids).toContain(SharedCorpus.bobDocId);

			expectHeader(response, "Content-Type", "application/json; charset=utf-8");
		},
	);

	test("every hit carries a real ranking score", { tag: "@contract" }, async ({ rest }) => {
		// The regression this catches and `jsonpath "$.hits[0].score" isNumber`
		// could not: `newBaseSearchRequest` sets `ShowRankingScore: true`, and
		// without it `getFloat64(hit, "_rankingScore")` falls back to `0.0` —
		// still a number, still passes the old assertion, and silently breaks
		// every consumer that ranks or thresholds on the score.
		const body = await expectJsonStatus(
			await rest.get(searchPath({ q: SharedCorpus.rustQuery, limit: "10" })),
			200,
			nonEmptySearchResponseSchema,
		);
		for (const hit of body.hits) {
			expect(hit.score, `${hit.id} has no ranking score`).toBeGreaterThan(0);
		}
	});

	test("limit caps the page and total counts it", { tag: "@contract" }, async ({ rest }) => {
		// Parity with 03-search-pagination-limit.hurl. The point of the scenario
		// is that `total` is `len(hits)` and NOT Meilisearch's
		// `estimatedTotalHits`: with two documents matching and `limit=1`, a
		// handler reporting the true total would answer `total: 2`.
		// `searchResponseSchema` enforces the `total == hits.length` invariant on
		// every response this suite parses; this test pins the specific number.
		const body = await expectJsonStatus(
			await rest.get(searchPath({ q: SharedCorpus.rustQuery, limit: "1" })),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits).toHaveLength(1);
		expect(body.total).toBe(1);
	});

	/**
	 * Parity with 06-search-limit-bounds.hurl. `rest/handler.go` accepts a limit
	 * only when `Atoi` succeeds *and* `0 < l <= 1000`; anything else silently
	 * falls back to 50. That permissiveness is deliberate (internal callers pass
	 * limits through from further upstream) but it is invisible from the
	 * outside, so it is pinned rather than assumed.
	 */
	for (const [label, limit] of [
		["zero", "0"],
		["above the 1000 ceiling", "1001"],
		["not a number at all", "notanumber"],
	] as const) {
		test(
			`a limit that is ${label} falls back to the default of 50`,
			{ tag: "@contract" },
			async ({ rest }) => {
				const body = await expectJsonStatus(
					await rest.get(searchPath({ q: SharedCorpus.rustQuery, limit })),
					200,
					nonEmptySearchResponseSchema,
				);
				// 50 > 2, so the whole match set comes back. A handler that had
				// started rejecting these would 400; one that passed `0` through
				// to Meilisearch would answer zero hits.
				expect(body.total).toBe(SharedCorpus.rustHitCount);
			},
		);
	}

	test(
		"a worker's own corpus round-trips tags, language and published_at",
		{ tag: "@contract" },
		async ({ rest, corpus }) => {
			// New coverage. The Hurl corpus set none of `language` or
			// `published_at`, so three of the seven fields in
			// `rest.SearchArticlesHit` were never observed populated —
			// `language` and `published_at` are `omitempty`, so "absent" and
			// "dropped on the floor by the driver" looked identical.
			//
			// `driver.hitsToDocs` reads them out of the Meilisearch hit and
			// `gateway.publishedAtFromUnix` converts the stored Unix seconds
			// back to a `time.Time` that the handler formats as RFC3339. Every
			// step of that only exists on this path.
			const body = await expectJsonStatus(
				await rest.get(searchPath({ q: corpus.nonce, limit: "10" })),
				200,
				nonEmptySearchResponseSchema,
			);

			const ownIds = new Set(corpus.docs.map((doc) => doc.id));
			expect(
				body.hits.map((hit) => hit.id).sort(),
				"a hit from outside this worker's corpus means the nonce is not unique, " +
					"and every count assertion in this suite is unreliable",
			).toEqual([...ownIds].sort());

			const first = body.hits.find((hit) => hit.id === corpus.docs[0]?.id);
			expect(first, "the document seeded at index 0 is missing").toBeDefined();
			expect(first?.tags).toEqual(["e2e", corpus.nonce]);
			// Seeded only on docs[0]; `json:"language,omitempty"` means the
			// others must omit the key rather than send an empty string.
			expect(first?.language).toBe("en");
			expect(first?.published_at).toBe(publishedAtRFC3339(0));

			const second = body.hits.find((hit) => hit.id === corpus.docs[1]?.id);
			expect(second?.language, "language must be absent, not empty").toBeUndefined();
			expect(second?.published_at).toBe(publishedAtRFC3339(1));
		},
	);

	test("content is served from the Meilisearch crop", { tag: "@contract" }, async ({
		rest,
		corpus,
	}) => {
		// `newBaseSearchRequest` excludes `content` from `AttributesToRetrieve`
		// and asks Meilisearch to crop it into `_formatted.content` instead, so
		// the driver never ships the full body. `getCropped` reads that, and its
		// own unit test records a production incident where the whole thing was
		// being discarded and every hit came back with `content: ""` —
		// indistinguishable from a document with no body, and perfectly
		// acceptable to Hurl's `isString`.
		const body = await expectJsonStatus(
			await rest.get(searchPath({ q: corpus.nonce, limit: "10" })),
			200,
			nonEmptySearchResponseSchema,
		);
		for (const hit of body.hits) {
			expect(hit.content, `${hit.id} came back with no content`).not.toBe("");
		}
	});
});

test.describe("user-scoped search (SearchByUserUsecase)", () => {
	/**
	 * Parity with 04-search-user-scoped.hurl, which ran the three cases as
	 * three entries in one file. They are three tests here because none of them
	 * depends on another: the fixture corpus is seeded before any worker starts
	 * and no test mutates it.
	 */
	for (const { user, docId } of [
		{ user: SharedCorpus.aliceUser, docId: SharedCorpus.aliceDocId },
		{ user: SharedCorpus.bobUser, docId: SharedCorpus.bobDocId },
	] as const) {
		test(`user_id=${user} sees only their own document`, { tag: "@authz" }, async ({ rest }) => {
			const body = await expectJsonStatus(
				await rest.get(searchPath({ q: SharedCorpus.rustQuery, user_id: user })),
				200,
				nonEmptySearchResponseSchema,
			);
			expect(body.hits).toHaveLength(1);
			expect(body.hits[0]?.id).toBe(docId);
			expect(body.total).toBe(1);
			// The other user's document matches the same query and is excluded
			// only by `BuildUserFilter`. Naming it makes this a leakage
			// assertion rather than a count assertion.
			expect(body.hits.map((hit) => hit.id)).not.toContain(
				user === SharedCorpus.aliceUser ? SharedCorpus.bobDocId : SharedCorpus.aliceDocId,
			);
		});
	}

	test("an unknown user gets an empty result set, not an error", { tag: "@contract" }, async ({
		rest,
	}) => {
		// Parity with the third entry of 04. It matters that this is a 200: the
		// callers of this endpoint treat a non-2xx as a search-backend outage
		// and retry, so answering 404 for "this user has nothing" would turn an
		// ordinary empty state into a retry storm.
		const body = await expectJsonStatus(
			await rest.get(
				searchPath({ q: SharedCorpus.rustQuery, user_id: SharedCorpus.unknownUser }),
			),
			200,
			searchResponseSchema,
		);
		expect(body.hits).toHaveLength(0);
		expect(body.total).toBe(0);
	});

	test("scopes to the caller's own corpus", { tag: "@authz" }, async ({ rest, corpus }) => {
		// New coverage, self-seeded: the shared-corpus cases above assert one
		// document each, which cannot distinguish "the filter works" from "the
		// query only ever matched one thing". Five documents under one
		// `user_id`, and a query that matches all five, does.
		const body = await expectJsonStatus(
			await rest.get(searchPath({ q: corpus.nonce, user_id: corpus.userId })),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits.map((hit) => hit.id).sort()).toEqual(
			corpus.docs.map((doc) => doc.id).sort(),
		);
	});

	test("a user id that owns nothing sees nothing", { tag: "@authz" }, async ({
		rest,
		corpus,
	}) => {
		// The tenant negative with a query that definitely has matches in the
		// index. `corpus.foreignUserId` is derived from the same nonce but is
		// not the owner, so a filter that was dropped — or applied to the wrong
		// attribute — returns all five documents here.
		const body = await expectJsonStatus(
			await rest.get(searchPath({ q: corpus.nonce, user_id: corpus.foreignUserId })),
			200,
			searchResponseSchema,
		);
		expect(body.hits).toHaveLength(0);
	});
});

test.describe("query validation", () => {
	/**
	 * Parity with 05-search-empty-query.hurl. Both spellings hit the same
	 * `query == ""` guard at the top of `SearchArticles`, before any usecase
	 * runs, which is why they are the only two inputs that produce a *400*.
	 */
	for (const [label, path] of [
		["omitted entirely", "/v1/search"],
		["present but empty", "/v1/search?q="],
	] as const) {
		test(`q ${label} is a 400`, { tag: "@contract" }, async ({ rest }) => {
			const response = await rest.get(path);
			await expectStatus(response, 400);
			expect(await response.text()).toContain("query parameter required");
			// `http.Error` writes plain text, not the JSON envelope the success
			// path uses. Clients that `JSON.parse` unconditionally break on this,
			// so the content type is part of the contract whether anyone likes it
			// or not.
			expectHeader(response, "Content-Type", "text/plain; charset=utf-8");
		});
	}

	/**
	 * New coverage: the inputs `usecase.validateAndSanitizeQuery` rejects.
	 *
	 * `rest/handler.go` funnels *every* usecase error — validation and
	 * Meilisearch outage alike — into one `http.Error(…, 500)`, so these do not
	 * answer 400 today. The status band below is not a shrug:
	 *
	 *   500  what the handler returns now, because the usecase's typed
	 *        validation errors are not classified before the response is
	 *        written.
	 *   400  what it should return, and what a future fix would return. This
	 *        test must not have to change to allow that fix.
	 *
	 * What must never happen is a **200**: that would mean control bytes, a
	 * 1000+ character query or a query that sanitises to nothing reached
	 * Meilisearch, which is precisely what the validator exists to prevent.
	 */
	for (const [label, q] of [
		["a control character", "ab\u0001c"],
		["a zero-width character", "ab\u200Bc"],
		["only whitespace", "   "],
		["more than 1000 characters", "a".repeat(1_001)],
	] as const) {
		test(`a query containing ${label} is refused`, { tag: "@contract" }, async ({ rest }) => {
			const response = await rest.get(searchPath({ q }));
			await expectStatusIn(response, [400, 500]);
		});
	}

	test(
		"a query of exactly 1000 characters is accepted and echoed sanitised",
		{ tag: "@contract" },
		async ({ rest }) => {
			// The other side of the boundary, and a second fact in the same
			// request.
			//
			// Boundary: the guard is `len(query) > 1000` on the *raw* value,
			// before sanitisation, so exactly 1000 characters is legal. Without
			// this, the rejection above would still pass if someone tightened the
			// check to `>= 1000` and started refusing valid queries.
			//
			// Echo: `SearchResult.Query` carries the *sanitised* query, not the
			// one the caller sent — `validateAndSanitizeQuery` NFC-normalises,
			// strips zero-width runes and folds whitespace runs. Padding a short
			// term out to exactly 1000 characters exercises both at once and pins
			// that callers are told what was actually searched for rather than
			// what they typed.
			const term = "zzqqxxnothingmatches";
			const body = await expectJsonStatus(
				await rest.get(searchPath({ q: term.padEnd(1_000, " ") })),
				200,
				searchResponseSchema,
			);
			expect(body.query).toBe(term);
		},
	);
});
