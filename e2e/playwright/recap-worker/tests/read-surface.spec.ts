import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { indexableGenresSchema, morningUpdatesSchema } from "../src/schemas.js";

/**
 * The read surface other services poll — entirely new coverage.
 *
 * The Hurl suite drove two of recap-worker's twenty-four routes and listed the
 * rest as "out of scope (deferred phases)". Two of the untested ones are
 * cross-service contracts with a scheduled consumer on the other end:
 *
 *   /v1/recaps/genres/indexable   search-indexer walks this to build the
 *                                 Meilisearch recap index.
 *   /v1/morning/updates           the morning surface's incremental feed.
 *
 * A contract nobody exercises is a contract that drifts. Both are asserted
 * here against schemas rather than against `exists`, and both have their
 * *parameter* behaviour pinned — because the interesting failure in a polled
 * endpoint is never the happy path, it is what happens to the cursor.
 *
 * Everything in this file is safe under `fullyParallel`: no assertion depends
 * on how many rows exist, only on shape and on relationships that hold at any
 * population. That is deliberate — the `pipeline` project may be writing
 * recaps at the same moment.
 */

test.describe("indexable genres (the search-indexer contract)", () => {
	test("GET /v1/recaps/genres/indexable answers the paging envelope @contract", async ({
		api,
	}) => {
		// api/fetch.rs:320-378. The envelope is `{results, has_more}`, not a
		// bare array — search-indexer's loop reads `has_more` to decide whether
		// to issue another page, so an endpoint that started returning a naked
		// list would make it index exactly one page forever and report success.
		const response = await api.get("/v1/recaps/genres/indexable");
		const body = await expectJsonStatus(response, 200, indexableGenresSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
		expect(typeof body.has_more).toBe("boolean");
	});

	test("has_more agrees with the page it was computed from @contract", async ({ api }) => {
		// `let has_more = hits.len() == limit` (api/fetch.rs:333). Asking for
		// one row makes the two observable together, at any population:
		//
		//   0 rows returned → has_more must be false  (no next page)
		//   1 row  returned → has_more must be true   (page is full)
		//
		// The off-by-one this catches is the one that matters: `>` instead of
		// `==` makes has_more permanently false and search-indexer silently
		// indexes the first page only; `>=` on an over-fetch makes it
		// permanently true and the loop never terminates.
		//
		// The limit=1 form only discriminates once a row exists, and this
		// project cannot guarantee one: it depends on `cold-start` (which
		// asserts an empty database) and runs alongside `pipeline` rather than
		// after it. So the `limit=0` probe below carries the falsifiable half.
		const body = await expectJsonStatus(
			await api.get("/v1/recaps/genres/indexable?limit=1"),
			200,
			indexableGenresSchema,
		);
		expect(body.results.length).toBeLessThanOrEqual(1);
		expect(body.has_more).toBe(body.results.length === 1);

		// `limit=0` states the same rule at a population every run has: `LIMIT
		// 0` selects nothing, and nothing *is* a full page, so `hits.len() ==
		// limit` holds and has_more must be true. That is a quirk being pinned
		// rather than endorsed — but it is the one form of this assertion that
		// fails on an empty database when someone rewrites the comparison as
		// `>`, which is the variant that costs search-indexer every page after
		// the first.
		const emptyPage = await expectJsonStatus(
			await api.get("/v1/recaps/genres/indexable?limit=0"),
			200,
			indexableGenresSchema,
		);
		expect(emptyPage.results).toEqual([]);
		expect(
			emptyPage.has_more,
			"a page requested with limit=0 is full at zero rows: hits.len() == limit",
		).toBe(true);
	});

	test("a future `since` returns nothing @contract", async ({ api }) => {
		// The incremental-sync contract: `since` filters on `executed_at`, so a
		// cursor in the future must select nothing regardless of what the
		// pipeline project is doing concurrently. A handler that dropped the
		// predicate would re-index the whole corpus on every poll — expensive
		// and invisible, because every row it returns is a valid row.
		const future = new Date(Date.now() + 86_400_000).toISOString();
		const body = await expectJsonStatus(
			await api.get(`/v1/recaps/genres/indexable?since=${encodeURIComponent(future)}`),
			200,
			indexableGenresSchema,
		);
		expect(body.results).toEqual([]);
		expect(body.has_more).toBe(false);
	});

	test("an unparseable `since` is silently ignored rather than rejected @contract", async ({
		api,
	}) => {
		// This pins a behaviour rather than endorsing it. `since` is
		// `Option<String>` parsed by hand with
		// `chrono::DateTime::parse_from_rfc3339(s).ok()` (api/fetch.rs:325-330),
		// and `.ok()` turns a malformed cursor into `None` — i.e. "no lower
		// bound", i.e. a full-corpus scan — with no signal to the caller.
		//
		// It is asserted because it is load-bearing in the wrong direction: a
		// search-indexer bug that corrupted its stored cursor would present as
		// a slow full reindex rather than an error, and this test is where the
		// decision to change it to a 400 would have to be made deliberately.
		// The neighbouring /v1/morning/updates test asserts the opposite choice
		// on the same kind of parameter.
		//
		// A status alone would pass just as happily if the malformed cursor were
		// coerced to "now" and filtered everything out — which is the one thing
		// that changes if somebody "fixes" the `.ok()`. So the malformed call is
		// compared with the unfiltered one: "no lower bound" means it must serve
		// at least what a call with no `since` at all serves. Issued in this
		// order deliberately — the `pipeline` project may commit a row between
		// the two requests, and a row that lands in between can only widen the
		// second result, never the first.
		const good = await expectJsonStatus(
			await api.get("/v1/recaps/genres/indexable"),
			200,
			indexableGenresSchema,
		);
		const bad = await expectJsonStatus(
			await api.get("/v1/recaps/genres/indexable?since=not-a-timestamp"),
			200,
			indexableGenresSchema,
		);
		const key = (hit: { job_id: string; genre: string }): string =>
			`${hit.job_id}:${hit.genre}`;
		expect(
			bad.results.map(key),
			"a malformed `since` must mean no lower bound, not a cursor at now",
		).toEqual(expect.arrayContaining(good.results.map(key)));
	});

	test("limit is clamped rather than trusted @contract", async ({ api }) => {
		// `query.limit.unwrap_or(200).min(1000)` (api/fetch.rs:324). The clamp
		// is what stops a caller asking for a million rows and holding a DB
		// connection for the duration; there is no way to observe the ceiling
		// from outside on a small table, so what is asserted is that an
		// over-large limit is *accepted* (clamped) rather than erroring —
		// changing the clamp to a rejection would break search-indexer's
		// existing callers.
		await expectJsonStatus(
			await api.get("/v1/recaps/genres/indexable?limit=100000"),
			200,
			indexableGenresSchema,
		);
	});
});

test.describe("morning updates", () => {
	test("GET /v1/morning/updates answers a bare array @contract", async ({ api }) => {
		// api/fetch.rs:659-693. Unlike the indexable endpoint next door, this
		// one returns a naked JSON array with no paging envelope. Pinning the
		// difference matters because the two are consumed by the same client
		// code path and a "make them consistent" refactor would break whichever
		// one it did not update.
		await expectJsonStatus(await api.get("/v1/morning/updates"), 200, morningUpdatesSchema);
	});

	test("a future `since` returns nothing @contract", async ({ api }) => {
		const future = new Date(Date.now() + 86_400_000).toISOString();
		const body = await expectJsonStatus(
			await api.get(`/v1/morning/updates?since=${encodeURIComponent(future)}`),
			200,
			morningUpdatesSchema,
		);
		expect(body).toEqual([]);
	});

	test("an unparseable `since` is rejected, unlike the indexable feed @contract", async ({
		api,
	}) => {
		// `MorningUpdatesQuery.since` is a typed
		// `Option<chrono::DateTime<Utc>>` (api/fetch.rs:644-647), so serde
		// fails the whole query struct and axum's `Query` extractor answers 400
		// before the handler runs.
		//
		// The point of this test is the contrast with
		// `/v1/recaps/genres/indexable?since=not-a-timestamp`, three tests up,
		// which answers 200 and quietly scans everything. Same parameter name,
		// same semantics, opposite failure behaviour — a genuine inconsistency
		// in the API that is now written down instead of being rediscovered by
		// whoever next writes a client.
		await expectStatus(await api.get("/v1/morning/updates?since=not-a-timestamp"), 400);
	});

	// The 24h default — `query.since.unwrap_or_else(|| Utc::now() -
	// Duration::hours(24))`, api/fetch.rs:663-665 — is deliberately NOT
	// asserted here, and this comment is the record of why rather than an
	// oversight to be re-filled with a passing test.
	//
	// `morning_article_groups` has exactly one writer,
	// `save_morning_article_groups` (pipeline/morning.rs:182), reachable only
	// from `Scheduler::run_morning_update` — which this slice never calls: the
	// morning daemon is off (`MORNING_DAEMON_ENABLED` unset, main.rs:174-175)
	// and `POST /v1/morning/letters/regenerate` is only probed for mounting
	// (tests/validation.spec.ts). Every response above is `[]`, so any
	// comparison between a defaulted and an explicit window is 0 vs 0 — and
	// even with rows, comparing two lengths cannot tell a 24h default from an
	// epoch default, which is the claim worth making.
	//
	// Testing it needs a fixture that inserts `morning_article_groups` rows
	// with `created_at` on both sides of the boundary, which needs DB access
	// this suite does not have. Until then the falsifiable half is the
	// parameter's rejection behaviour, asserted above.
});
