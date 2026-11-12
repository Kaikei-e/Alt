import { test, expect } from "../src/fixtures.js";
import { callUnary } from "../../_shared/connect.js";
import { expectStatus } from "../../_shared/http.js";

const SVC = "/services.datahub.v1.DataHubService";

/**
 * Procedures removed from the `alt-backend` peer allowlist in
 * `alt-backend/app/middleware/require_peer_procedure.go` (owned by background
 * workers or unowned by any peer). Presenting `alt-backend`'s certificate must
 * be rejected with HTTP 403 before reaching the handler.
 */
const DENIED_FOR_ALT_BACKEND = [
	"GetLatestArticleTimestamp",
	"GetArticleByID",
	"CheckArticleExists",
	"CreateArticle",
	"GetEmptyFeedID",
	"CheckArticleSummaryExists",
	"FetchArticlesByTag",
	"ListRecapArticles",
	"RecordFeedLinkFailure",
	"GetLatestSummaryVersion",
	"GetTagSetVersionByID",
	"ListArticlesWithTags",
	"ListArticlesWithTagsForward",
	"ListDeletedArticles",
	"ListRecentArticles",
	"GetSystemUser",
	"ListFeedURLs",
	"ListUntaggedArticles",
	"BatchGetTagsByArticleIDs",
	"FindArticlesWithSummaries",
	"HasUnsummarizedArticles",
	"ListUnsummarizedArticles",
	"ListFeedsInWindow",
	"ClaimOutboxBatch",
	"GetNotificationBacklogAge",
	"ListPushSubscriptionsForUser",
] as const;

test.describe("per-peer DataHubService capability boundaries", () => {
	test("an allowed pre-processor TLS peer cannot invoke backend-only mutations", { tag: "@authz" }, async ({
		peers,
	}) => {
		await expectStatus(
			await callUnary(peers["pre-processor"], `${SVC}/GetSystemUser`, {}),
			200,
		);
		const denied = await callUnary(peers["pre-processor"], `${SVC}/DeleteFeedLink`, {
			id: "00000000-0000-4000-a000-000000000001",
		});
		await expectStatus(denied, 403);
		expect(await denied.text()).toContain("forbidden: peer not authorized for this procedure");
	});

	for (const procedure of DENIED_FOR_ALT_BACKEND) {
		test(`alt-backend peer is denied ${procedure} → 403`, { tag: "@authz" }, async ({ peers }) => {
			const denied = await callUnary(peers["alt-backend"], `${SVC}/${procedure}`, {});
			await expectStatus(denied, 403);
			expect(await denied.text()).toContain("forbidden: peer not authorized for this procedure");
		});
	}
});
