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
 * GET /v1/search requires both query `q` and tenant `user_id`.
 * Results are always tenant-filtered via `SearchByUserUsecase.ExecuteWithPagination`.
 * Calls omitting `user_id` are rejected with HTTP 400 `user_id parameter required`.
 */

/** `?q=…` with the parameters a scenario cares about, in a readable form. */
function searchPath(params: Record<string, string>): string {
	const query = new URLSearchParams(params).toString();
	return `/v1/search?${query}`;
}

test.describe("tenant-scoped search (SearchByUserUsecase)", () => {
	test(
		"returns the seeded matches with a complete hit envelope",
		{ tag: "@smoke" },
		async ({ rest }) => {
			// Parity with 02-search-basic.hurl, updated for mandatory tenant scoping.
			// The schema covers every hit. Assert that Alice sees her document and
			// does not see Bob's document (tenant isolation).
			const response = await rest.get(
				searchPath({
					q: SharedCorpus.rustQuery,
					user_id: SharedCorpus.aliceUser,
					limit: "10",
				}),
			);
			const body = await expectJsonStatus(response, 200, nonEmptySearchResponseSchema);

			expect(body.query).toBe(SharedCorpus.rustQuery);
			expect(body.hits.length).toBeGreaterThanOrEqual(SharedCorpus.aliceRustHitCount);

			const ids = body.hits.map((hit) => hit.id);
			expect(ids).toContain(SharedCorpus.aliceDocId);
			expect(ids).not.toContain(SharedCorpus.bobDocId);

			expectHeader(response, "Content-Type", "application/json; charset=utf-8");
		},
	);

	test("every hit carries a real ranking score", { tag: "@contract" }, async ({ rest }) => {
		// `newBaseSearchRequest` sets `ShowRankingScore: true`, and
		// without it `getFloat64(hit, "_rankingScore")` falls back to `0.0`.
		const body = await expectJsonStatus(
			await rest.get(
				searchPath({
					q: SharedCorpus.rustQuery,
					user_id: SharedCorpus.aliceUser,
					limit: "10",
				}),
			),
			200,
			nonEmptySearchResponseSchema,
		);
		for (const hit of body.hits) {
			expect(hit.score, `${hit.id} has no ranking score`).toBeGreaterThan(0);
		}
	});

	test("limit caps the page and total counts it", { tag: "@contract" }, async ({ rest, corpus }) => {
		// Parity with 03-search-pagination-limit.hurl.
		// `total` is `len(hits)` and NOT Meilisearch's `estimatedTotalHits`.
		// Using the worker's 5-document corpus with limit=2 pins that total == 2.
		const body = await expectJsonStatus(
			await rest.get(
				searchPath({
					q: corpus.nonce,
					user_id: corpus.userId,
					limit: "2",
				}),
			),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits).toHaveLength(2);
		expect(body.total).toBe(2);
	});

	/**
	 * Parity with 06-search-limit-bounds.hurl. `rest/handler.go` accepts a limit
	 * only when `Atoi` succeeds *and* `0 < l <= 1000`; anything else silently
	 * falls back to 20.
	 */
	for (const [label, limit] of [
		["zero", "0"],
		["above the 1000 ceiling", "1001"],
		["not a number at all", "notanumber"],
	] as const) {
		test(
			`a limit that is ${label} falls back to the default of 20`,
			{ tag: "@contract" },
			async ({ rest }) => {
				const body = await expectJsonStatus(
					await rest.get(
						searchPath({
							q: SharedCorpus.rustQuery,
							user_id: SharedCorpus.aliceUser,
							limit,
						}),
					),
					200,
					nonEmptySearchResponseSchema,
				);
				expect(body.total).toBe(SharedCorpus.aliceRustHitCount);
			},
		);
	}

	test(
		"a worker's own corpus round-trips tags, language and published_at",
		{ tag: "@contract" },
		async ({ rest, corpus }) => {
			const body = await expectJsonStatus(
				await rest.get(
					searchPath({
						q: corpus.nonce,
						user_id: corpus.userId,
						limit: "10",
					}),
				),
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
		const body = await expectJsonStatus(
			await rest.get(
				searchPath({
					q: corpus.nonce,
					user_id: corpus.userId,
					limit: "10",
				}),
			),
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

test.describe("parameter validation", () => {
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
	 * Regression coverage for mandatory `user_id` parameter on GET /v1/search.
	 * `userID == ""` guard immediately follows the query guard.
	 */
	for (const [label, params] of [
		["omitted entirely", { q: SharedCorpus.rustQuery }],
		["present but empty", { q: SharedCorpus.rustQuery, user_id: "" }],
		["only whitespace", { q: SharedCorpus.rustQuery, user_id: "   " }],
	] as const) {
		test(`user_id ${label} is a 400`, { tag: ["@contract", "@authz"] }, async ({ rest }) => {
			const response = await rest.get(searchPath(params));
			await expectStatus(response, 400);
			expect(await response.text()).toContain("user_id parameter required");
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
			const response = await rest.get(
				searchPath({ q, user_id: SharedCorpus.aliceUser }),
			);
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
				await rest.get(
					searchPath({
						q: term.padEnd(1_000, " "),
						user_id: SharedCorpus.aliceUser,
					}),
				),
				200,
				searchResponseSchema,
			);
			expect(body.query).toBe(term);
		},
	);
});
