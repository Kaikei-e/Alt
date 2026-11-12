import { test, expect, probeArticleURL } from "../src/fixtures.js";
import type { SuitePeer } from "../src/env.js";
import { ConnectCode, callUnary, expectUnaryError } from "../../_shared/connect.js";
import { expectJson } from "../../_shared/http.js";
import { connectErrorSchema } from "../src/schemas.js";
import { uuid } from "../../_shared/ids.js";

/**
 * The error half of the contract — mostly new coverage.
 *
 * `04-datahub-service.hurl` carried exactly one negative: `within_hours: 0` →
 * `400` with `jsonpath "$.code" == "invalid_argument"`, and its comment said
 * why it mattered — "bad input must reach the handler's validation rather than
 * the router, so that a rejection proves the mount as firmly as an acceptance
 * does".
 *
 * That argument generalises, and this file is it generalised. Two things make
 * it worth more here than on a typical service:
 *
 *   1. **404 is ambiguous on this listener.** `cmd/datahub`'s router answers
 *      anything outside `/services.datahub.v1.` with `http.NotFound`, and
 *      connect-go answers an unknown procedure the same way. A `not_found`
 *      *business* answer therefore has to be told apart from a missing mount
 *      by its body — a Connect envelope versus Go's plain text — which is a
 *      distinction no status-only assertion can make.
 *
 *   2. **These are the codes generated clients switch on.** connect-es
 *      surfaces `ConnectError.code` as a numeric enum derived from the wire
 *      string; a handler that answered `internal` where it means
 *      `invalid_argument` sends every caller's error handling to its default
 *      branch (bp-typescript rule 10). `expectUnaryError` asserts the code and
 *      the HTTP status the Connect protocol pairs it with, so a handler that
 *      gets one right and the other wrong still fails.
 */

const SVC = "/services.datahub.v1.DataHubService";

/**
 * Required-field validations, taken from the handler sources. Each is a guard
 * that runs *before* any database access, so the expected answer does not
 * depend on what is in the tables.
 */
const REQUIRED_FIELD_CASES: ReadonlyArray<{
	procedure: string;
	peer: SuitePeer;
	request: Record<string, unknown>;
	field: string;
}> = [
	// handler.go
	{ procedure: "GetArticleByID", peer: "search-indexer", request: {}, field: "article_id" },
	{ procedure: "CheckArticleExists", peer: "pre-processor", request: { feedId: uuid() }, field: "url" },
	{ procedure: "CheckArticleExists", peer: "pre-processor", request: { url: "https://stub.invalid/x" }, field: "feed_id" },
	{ procedure: "CreateArticle", peer: "pre-processor", request: {}, field: "url" },
	{ procedure: "SaveArticleSummary", peer: "alt-backend", request: {}, field: "article_id" },
	{ procedure: "GetFeedID", peer: "alt-backend", request: {}, field: "feed_url" },
	{ procedure: "GetEmptyFeedID", peer: "pre-processor", request: {}, field: "feed_url" },
	{ procedure: "CheckArticleSummaryExists", peer: "pre-processor", request: {}, field: "article_id" },
	{ procedure: "FetchArticlesByTag", peer: "rag-orchestrator", request: {}, field: "tag_name" },
	{ procedure: "ListRecapArticles", peer: "recap-worker", request: {}, field: "from" },
	{ procedure: "ListRecapArticles", peer: "recap-worker", request: { from: "2026-01-01T00:00:00Z" }, field: "to" },
	// wave3_handler.go
	{ procedure: "GetArticleHead", peer: "alt-backend", request: {}, field: "article_id" },
	{ procedure: "GetImageProxyCache", peer: "alt-backend", request: {}, field: "url_hash" },
	{ procedure: "IsDomainDeclined", peer: "alt-backend", request: {}, field: "user_id/domain" },
	// wave3_batch2_handler.go
	{ procedure: "ArchiveArticle", peer: "alt-backend", request: {}, field: "url" },
	{ procedure: "GetArticleByURL", peer: "alt-backend", request: {}, field: "url" },
	{ procedure: "ListArticlesCursor", peer: "alt-backend", request: {}, field: "user_id" },
	// wave3_batch3_handler.go
	{ procedure: "RecordFeedLinkFailure", peer: "alt-harvester", request: {}, field: "feed_url" },
	{ procedure: "ListFeedsCursor", peer: "alt-backend", request: { userId: uuid() }, field: "scope" },
	// wave3_batch4_handler.go
	{ procedure: "GetReadFeedIDs", peer: "alt-backend", request: {}, field: "user_id" },
	{ procedure: "GetArticleTags", peer: "alt-backend", request: {}, field: "article_id" },
	{ procedure: "SearchTagsByPrefix", peer: "alt-backend", request: {}, field: "prefix" },
	// wave3_batch6_handler.go
	{ procedure: "ListArticlesByTagID", peer: "alt-backend", request: {}, field: "tag_id" },
	{ procedure: "GetArticleTitleAndLink", peer: "alt-backend", request: {}, field: "article_id" },
];

test.describe("required-field validation", () => {
	for (const { procedure, peer, request, field } of REQUIRED_FIELD_CASES) {
		test(`${procedure} rejects a missing ${field}`, { tag: "@contract" }, async ({ peers }) => {
			await expectUnaryError(
				peers[peer],
				`${SVC}/${procedure}`,
				request,
				ConnectCode.invalidArgument,
			);
		});
	}
});

test.describe("malformed-value validation", () => {
	test("ListRecentArticles rejects a non-positive within_hours", { tag: "@contract" }, async ({
		peers,
	}) => {
		// The one negative the Hurl suite had. It is not redundant with the
		// required-field cases above: `within_hours` is *optional*, and the
		// handler reproduces the retired REST route's rejection rather than
		// deferring to the usecase, which clamps `<= 0` silently to 24
		// (fetch_recent_articles_usecase.go). Without this assertion a caller
		// asking for an impossible window would get a quietly different one.
		await expectUnaryError(
			peers["rag-orchestrator"],
			`${SVC}/ListRecentArticles`,
			{ withinHours: 0 },
			ConnectCode.invalidArgument,
		);
	});

	test("ListRecentArticles rejects a negative limit", { tag: "@contract" }, async ({ peers }) => {
		// Same guard, other field, same reason: the usecase would clamp a
		// negative limit to 100 without saying so.
		await expectUnaryError(
			peers["rag-orchestrator"],
			`${SVC}/ListRecentArticles`,
			{ limit: -1 },
			ConnectCode.invalidArgument,
		);
	});

	test("ListRecapArticles rejects a non-RFC3339 range", { tag: "@contract" }, async ({ peers }) => {
		// `from`/`to` are strings on the wire, parsed with `time.Parse(RFC3339)`.
		// A parse failure that came back as `internal` would tell recap-worker
		// "the provider is broken" when it means "your request is".
		await expectUnaryError(
			peers["recap-worker"],
			`${SVC}/ListRecapArticles`,
			{ from: "yesterday", to: "today" },
			ConnectCode.invalidArgument,
		);
	});

	test("ListArticlesCursor rejects a user_id that is not a UUID", { tag: "@contract" }, async ({
		peers,
	}) => {
		// `requiredUUID` (wave3_batch2_handler.go) separates "absent" from
		// "present but unparseable" and answers invalid_argument for both. The
		// second is the one worth a test: a caller passing a Kratos identity
		// *email* instead of its id must not reach a query that would then fail
		// as a 500 and look like a database fault.
		await expectUnaryError(
			peers["alt-backend"],
			`${SVC}/ListArticlesCursor`,
			{ userId: "not-a-uuid" },
			ConnectCode.invalidArgument,
		);
	});
});

test.describe("not_found is a business answer, not a missing mount", () => {
	test("GetFeedID answers a Connect not_found envelope for an unknown feed", { tag: "@contract" }, async ({
		peers,
		token,
	}) => {
		// This is the assertion that only exists because 404 is overloaded on
		// this listener. `cmd/datahub`'s router 404s an unrecognised prefix and
		// connect-go 404s an unknown procedure, both with Go's plain-text
		// `404 page not found`; a *mounted* procedure answering `not_found`
		// returns the same status with a JSON Connect envelope.
		//
		// So the body is the discriminator, and without it "GetFeedID was
		// deleted from the proto" and "that feed does not exist" are the same
		// observation. The URL carries this test's token, so no sibling worker
		// can have registered it.
		const response = await callUnary(peers["alt-backend"], `${SVC}/GetFeedID`, {
			feedUrl: probeArticleURL(token),
		});
		expect(response.status()).toBe(404);

		const body = await expectJson(response, connectErrorSchema);
		expect(
			body.code,
			"a 404 with no Connect envelope means the procedure is not mounted at all",
		).toBe(ConnectCode.notFound);
	});

	test("GetArticleByID answers a Connect not_found envelope for an unknown id", { tag: "@contract" }, async ({
		peers,
	}) => {
		// Same discriminator on the read path search-indexer uses. A freshly
		// minted v4 UUID cannot collide with a seeded row or a sibling worker's.
		const response = await callUnary(peers["search-indexer"], `${SVC}/GetArticleByID`, {
			articleId: uuid(),
		});
		expect(response.status()).toBe(404);
		const body = await expectJson(response, connectErrorSchema);
		expect(body.code).toBe(ConnectCode.notFound);
	});
});
