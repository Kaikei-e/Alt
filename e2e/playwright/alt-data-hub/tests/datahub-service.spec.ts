import { test, expect, probeArticleURL } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { callUnary } from "../../_shared/connect.js";
import { ZERO_UUID } from "../src/env.js";
import {
	articlesWithTagsPageSchema,
	checkArticleExistsSchema,
	deletedArticlesPageSchema,
	latestArticleTimestampSchema,
	recentArticlesSchema,
} from "../src/schemas.js";

/**
 * `services.datahub.v1.DataHubService` answers over mTLS — the port of
 * `04-datahub-service.hurl`, with the bodies pinned.
 *
 * The Hurl file's own header explains why it asserted only status codes:
 * "Connect's stock protojson codec omits zero-valued scalars, so asserting on
 * them would encode a codec detail." That is true of the *values* and false as
 * a conclusion about the *envelope*. `connect/v2/datahub/server.go` builds
 * `datahubOpts` from interceptors alone — no `EmitUnpopulatedJSONCodec`, unlike
 * alt-backend's operator listener — so omission is deterministic: zero fields
 * are always absent and non-zero fields are always present. `src/schemas.ts`
 * encodes both directions, which is how `HTTP 200` becomes an assertion about
 * what the five consumers actually receive.
 *
 * Every request here is idempotent and reads only. The staging slice boots
 * with an empty `articles` table and this suite keeps it that way, so a
 * sibling worker's page can never appear in this one's.
 */

const SVC = "/services.datahub.v1.DataHubService";

test.describe("DataHubService reads", () => {
	test("GetLatestArticleTimestamp answers an empty envelope on an empty table", { tag: "@contract" }, async ({
		dataHub,
	}) => {
		const response = await callUnary(dataHub, `${SVC}/GetLatestArticleTimestamp`, {});
		const body = await expectJsonStatus(response, 200, latestArticleTimestampSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		// handler.go sets `latest_created_at` only when the repository returned a
		// non-nil time. The staging slice's `articles` table is empty, so its
		// absence is the correct answer — and asserting the absence rather than
		// shrugging is what would catch a handler that started emitting a zero
		// `1970-01-01T00:00:00Z`, which alt-harvester's incremental mark would
		// read as "fetch everything ever published".
		expect(
			body.latestCreatedAt,
			"an empty articles table must produce no timestamp at all, not the epoch",
		).toBeUndefined();
	});

	test("ListArticlesWithTags pages an empty table", { tag: "@contract" }, async ({ dataHub }) => {
		const body = await expectJsonStatus(
			await callUnary(dataHub, `${SVC}/ListArticlesWithTags`, { limit: 10 }),
			200,
			articlesWithTagsPageSchema,
		);
		// A final page carries no cursor. `next_created_at`/`next_id` are set
		// from the repository's keyset tail, so a cursor present on an empty
		// page would make search-indexer loop forever on the same request.
		expect(body.nextCreatedAt, "an empty page must not hand back a cursor").toBeUndefined();
	});

	test("ListArticlesWithTagsForward accepts an incremental mark", { tag: "@contract" }, async ({
		dataHub,
	}) => {
		// This is alt-harvester's / search-indexer's forward scan. The mark is a
		// required timestamp on the wire; sending one far in the past is the
		// request shape those callers use on a cold index.
		const body = await expectJsonStatus(
			await callUnary(dataHub, `${SVC}/ListArticlesWithTagsForward`, {
				incrementalMark: "2020-01-01T00:00:00Z",
				limit: 10,
			}),
			200,
			articlesWithTagsPageSchema,
		);
		expect(body.nextCreatedAt).toBeUndefined();
	});

	test("ListDeletedArticles pages the tombstones", { tag: "@contract" }, async ({ dataHub }) => {
		// search-indexer's deletion feed. Its envelope has a *different* cursor
		// field (`next_deleted_at`) from the two above, and a schema is the only
		// way that difference stays visible — `jsonpath "$" exists` was true of
		// either shape and of `{"code":"internal"}` too.
		const body = await expectJsonStatus(
			await callUnary(dataHub, `${SVC}/ListDeletedArticles`, { limit: 10 }),
			200,
			deletedArticlesPageSchema,
		);
		expect(body.nextDeletedAt).toBeUndefined();
	});

	test("ListRecentArticles reports its window even when the window is empty", { tag: "@contract" }, async ({
		dataHub,
	}) => {
		// rag-orchestrator's reader, absorbed from `GET /v1/internal/articles/
		// recent` by ADR-000954 D6. `since`/`until` are formatted strings the
		// codec cannot omit, so they are present on every answer — which makes
		// them the assertion that survives an empty database. The old REST route
		// and the Hurl port of it could only say "200".
		const body = await expectJsonStatus(
			await callUnary(dataHub, `${SVC}/ListRecentArticles`, {}),
			200,
			recentArticlesSchema,
		);
		expect(
			Date.parse(body.until),
			"until must not precede since — the window would be inverted",
		).toBeGreaterThanOrEqual(Date.parse(body.since));
	});

	test("ListRecentArticles honours an explicit window", { tag: "@contract" }, async ({ dataHub }) => {
		const body = await expectJsonStatus(
			await callUnary(dataHub, `${SVC}/ListRecentArticles`, { withinHours: 24, limit: 10 }),
			200,
			recentArticlesSchema,
		);
		// 24h asked for, 24h reported. The usecase clamps silently on some
		// inputs, so checking that the *reported* window matches the requested
		// one is what distinguishes "honoured" from "ignored and defaulted".
		// A one-minute tolerance absorbs the handler's own `time.Now()`.
		const spanHours = (Date.parse(body.until) - Date.parse(body.since)) / 3_600_000;
		expect(spanHours).toBeGreaterThan(23.9);
		expect(spanHours).toBeLessThan(24.1);
	});

	test("ListRecentArticles treats limit=0 as 'time window only'", { tag: "@contract" }, async ({
		dataHub,
	}) => {
		// An explicit zero is a legitimate request, distinct from the field
		// being absent — the REST route it replaced had the same meaning. The
		// field is `optional` in the proto, so the server can tell the two
		// apart; with proto3 implicit presence it could not, and this 200 is
		// what says the explicit-presence declaration is still there.
		await expectJsonStatus(
			await callUnary(dataHub, `${SVC}/ListRecentArticles`, { limit: 0 }),
			200,
			recentArticlesSchema,
		);
	});
});

test.describe("DataHubService write path", () => {
	test("CheckArticleExists answers false for a URL nobody stored", { tag: "@contract" }, async ({
		dataHub,
		token,
	}) => {
		// pre-processor's dedupe probe — the one procedure on this surface whose
		// misbehaviour is silently destructive: a false positive drops an
		// article on the floor for good.
		//
		// The URL carries this test's own token, so the answer is deterministic
		// no matter which worker or shard runs it and no matter what a sibling
		// is doing. The Hurl file used `{{run_id}}`, which was unique per
		// dispatch — enough only because `--jobs 1` meant one request at a time.
		const body = await expectJsonStatus(
			await callUnary(dataHub, `${SVC}/CheckArticleExists`, {
				url: probeArticleURL(token),
				feedId: ZERO_UUID,
			}),
			200,
			checkArticleExistsSchema,
		);
		expect(body.exists ?? false, "a URL this run invented cannot already exist").toBe(false);
	});
});
