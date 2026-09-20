import { expect, test } from "../src/fixtures.js";
import { expectHeader, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { publishedAtRFC3339 } from "../src/corpus.js";
import { nonEmptySearchResponseSchema, searchResponseSchema } from "../src/schemas.js";

/**
 * `published_after` / `published_before` on `GET /v1/search` — entirely new
 * coverage.
 *
 * The Hurl suite never sent either parameter, and could not have: its fixture
 * corpus carries no `published_at` at all, so a date window would have matched
 * nothing whether the filter worked or not. The whole path — the RFC3339 parse
 * in `rest/handler.go`, `SearchArticlesUsecase.ExecuteWithDateFilter`,
 * `MeilisearchDriver.SearchWithDateFilter` building
 * `published_at >= X AND published_at <= Y`, and the `published_at` entry in
 * `EnsureIndex`'s filterable attributes — was untested end to end.
 *
 * Each worker's corpus is five documents published on five consecutive days
 * from 2026-01-01 (`src/corpus.ts`), which is what makes an exact boundary
 * assertion possible: "three of five" is a claim a broken filter cannot
 * accidentally satisfy the way "at least one" can.
 *
 * Date filtering on GET /v1/search requires mandatory user_id and executes
 * through SearchByUserUsecase.ExecuteWithDateFilter.
 */

function searchPath(params: Record<string, string>): string {
	return `/v1/search?${new URLSearchParams(params).toString()}`;
}

/** Ids of the corpus documents at the given indices, sorted for comparison. */
function idsAt(docs: readonly { readonly id: string }[], indices: readonly number[]): string[] {
	return indices.map((index) => docs[index]?.id ?? `<missing index ${index}>`).sort();
}

test.describe("date-windowed search", () => {
	test("published_after keeps the boundary document", { tag: "@contract" }, async ({
		rest,
		corpus,
	}) => {
		// The clause is `published_at >= after.Unix()` — inclusive. Documents 2,
		// 3 and 4 qualify; 0 and 1 do not. Asserting the *set* rather than a
		// count is what makes the inclusivity visible: an exclusive `>` would
		// still return "some" documents and only this comparison notices which.
		const body = await expectJsonStatus(
			await rest.get(
				searchPath({
					q: corpus.nonce,
					user_id: corpus.userId,
					published_after: publishedAtRFC3339(2),
				}),
			),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits.map((hit) => hit.id).sort()).toEqual(idsAt(corpus.docs, [2, 3, 4]));
	});

	test("published_before keeps the boundary document", { tag: "@contract" }, async ({
		rest,
		corpus,
	}) => {
		// Mirror of the above: `published_at <= before.Unix()`, so 0, 1 and 2.
		const body = await expectJsonStatus(
			await rest.get(
				searchPath({
					q: corpus.nonce,
					user_id: corpus.userId,
					published_before: publishedAtRFC3339(2),
				}),
			),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits.map((hit) => hit.id).sort()).toEqual(idsAt(corpus.docs, [0, 1, 2]));
	});

	test("both bounds AND together into a window", { tag: "@contract" }, async ({
		rest,
		corpus,
	}) => {
		// `SearchWithDateFilter` joins the two clauses with " AND ". Joining
		// them with OR — or building only the last clause, which is what a
		// `filter = …` overwrite instead of an append looks like — returns four
		// or five documents here rather than three.
		const body = await expectJsonStatus(
			await rest.get(
				searchPath({
					q: corpus.nonce,
					user_id: corpus.userId,
					published_after: publishedAtRFC3339(1),
					published_before: publishedAtRFC3339(3),
				}),
			),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits.map((hit) => hit.id).sort()).toEqual(idsAt(corpus.docs, [1, 2, 3]));
	});

	test("an empty bound means absent, not epoch zero", { tag: "@contract" }, async ({
		rest,
		corpus,
	}) => {
		// `parseOptionalRFC3339("")` returns `(nil, nil)`, and with both bounds
		// nil the driver degrades to a plain `Search`. The alternative reading —
		// parse failure, or a zero `time.Time` used as a real bound — would be a
		// 400 and a filter of `published_at >= -6795364578` respectively, and
		// both are plausible enough regressions to fence.
		const body = await expectJsonStatus(
			await rest.get(
				searchPath({
					q: corpus.nonce,
					user_id: corpus.userId,
					published_after: "",
					published_before: "",
				}),
			),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits.map((hit) => hit.id).sort()).toEqual(
			idsAt(corpus.docs, [0, 1, 2, 3, 4]),
		);
	});
});

test.describe("date parameter validation", () => {
	for (const parameter of ["published_after", "published_before"] as const) {
		test(`a non-RFC3339 ${parameter} is a 400`, { tag: "@contract" }, async ({ rest, corpus }) => {
			// Both bounds are parsed after user_id validation and before search runs.
			const response = await rest.get(
				searchPath({
					q: "rust",
					user_id: corpus.userId,
					[parameter]: "2026-01-01",
				}),
			);
			await expectStatus(response, 400);
			expect(await response.text()).toContain(`invalid ${parameter} (expected RFC3339)`);
			expectHeader(response, "Content-Type", "text/plain; charset=utf-8");
		});
	}

	test(
		"published_after after published_before is a 400",
		{ tag: "@contract" },
		async ({ rest, corpus }) => {
			const response = await rest.get(
				searchPath({
					q: corpus.nonce,
					user_id: corpus.userId,
					published_after: publishedAtRFC3339(3),
					published_before: publishedAtRFC3339(1),
				}),
			);
			await expectStatus(response, 400);
			expect(await response.text()).toContain(
				"published_after must not be after published_before",
			);
			expectHeader(response, "Content-Type", "text/plain; charset=utf-8");
		},
	);

	test(
		"date bounds combined with user_id enforce both tenant isolation and date window",
		{ tag: ["@contract", "@authz"] },
		async ({ rest, corpus }) => {
			// Production supports scoped dates via SearchByUserUsecase.ExecuteWithDateFilter.
			// Assert positive intersection: corpus owner gets only documents within the date window.
			const body = await expectJsonStatus(
				await rest.get(
					searchPath({
						q: corpus.nonce,
						user_id: corpus.userId,
						published_after: publishedAtRFC3339(2),
					}),
				),
				200,
				nonEmptySearchResponseSchema,
			);
			expect(body.hits.map((hit) => hit.id).sort()).toEqual(idsAt(corpus.docs, [2, 3, 4]));

			// Assert tenant isolation: a foreign user id querying the same date window matches nothing.
			const foreign = await expectJsonStatus(
				await rest.get(
					searchPath({
						q: corpus.nonce,
						user_id: corpus.foreignUserId,
						published_after: publishedAtRFC3339(2),
					}),
				),
				200,
				searchResponseSchema,
			);
			expect(foreign.hits).toHaveLength(0);
			expect(foreign.total).toBe(0);
		},
	);

	test(
		"a date window that excludes everything is an empty 200",
		{ tag: "@contract" },
		async ({ rest, corpus }) => {
			// The corpus ends on 2026-01-05; a window opening the day after
			// matches nothing. It has to be a 200 with `total: 0` for the same
			// reason the unknown-user case does — callers treat non-2xx as a
			// search-backend outage and retry.
			const body = await expectJsonStatus(
				await rest.get(
					searchPath({
						q: corpus.nonce,
						user_id: corpus.userId,
						published_after: publishedAtRFC3339(5),
					}),
				),
				200,
				searchResponseSchema,
			);
			expect(body.hits).toHaveLength(0);
			expect(body.total).toBe(0);
		},
	);
});
